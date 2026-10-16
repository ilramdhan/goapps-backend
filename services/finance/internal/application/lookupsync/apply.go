package lookupsync

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FilledByLookupSync is recorded in cpp_filled_by for values written by a
// lookup sync (cpp_created_by / cpp_updated_by stay the requesting actor).
const FilledByLookupSync = "lookup_sync"

// SkipProductNotFound is reported when the product no longer exists at apply
// time.
const SkipProductNotFound = "PRODUCT_NOT_FOUND"

// ApplyTx is the transaction-scoped write port used by ApplyProduct. All calls
// run inside one database transaction opened by TxRunner.
type ApplyTx interface {
	// Reader returns a SyncReader bound to the transaction, so the re-plan
	// sees exactly the rows about to be written.
	Reader() SyncReader
	// LockProduct takes a row lock (SELECT ... FOR UPDATE) on the product
	// master row and returns its code and cpm_is_locked. found=false when the
	// product does not exist.
	LockProduct(ctx context.Context, productSysID int64) (code string, locked, found bool, err error)
	// UpsertNumeric / UpsertText write one cost_product_parameter value.
	UpsertNumeric(ctx context.Context, productSysID int64, paramID uuid.UUID, v decimal.Decimal, filledBy, actor string) error
	UpsertText(ctx context.Context, productSysID int64, paramID uuid.UUID, v, filledBy, actor string) error
	// DeleteValue removes the cost_product_parameter row (the applicable-param
	// row is untouched). deleted=false when no row existed.
	DeleteValue(ctx context.Context, productSysID int64, paramID uuid.UUID) (deleted bool, err error)
}

// TxRunner runs fn in a transaction: commit when fn returns nil, rollback
// otherwise.
type TxRunner interface {
	InTx(ctx context.Context, fn func(tx ApplyTx) error) error
}

// ProductApplyResult is the outcome of applying a sync to one product.
type ProductApplyResult struct {
	ProductSysID  int64
	ProductCode   string
	ValuesWritten int
	ValuesCleared int
	Skipped       bool
	// SkipReason is SkipLocked or SkipProductNotFound when Skipped.
	SkipReason string
}

// ProductApplier applies a lookup sync to one product.
type ProductApplier interface {
	ApplyProduct(ctx context.Context, productSysID int64, scope Scope, actor string) (ProductApplyResult, error)
}

// Applier implements ProductApplier: lock → re-plan in tx → write → commit.
type Applier struct {
	planner *Planner
	tx      TxRunner
}

var _ ProductApplier = (*Applier)(nil)

// NewApplier creates an Applier. planner supplies resolver + fill-group
// children; its reader is replaced per call by the tx-bound reader.
func NewApplier(planner *Planner, tx TxRunner) *Applier {
	return &Applier{planner: planner, tx: tx}
}

// ApplyProduct re-plans productSysID within scope inside one transaction,
// holding a FOR UPDATE lock on its product master row, and applies every
// applicable change: SET upserts with cpp_filled_by=FilledByLookupSync, CLEAR
// deletes the value row. A locked (or vanished) product is skipped with no
// writes. It never adds applicable params and never writes the trigger param.
func (a *Applier) ApplyProduct(ctx context.Context, productSysID int64, scope Scope, actor string) (ProductApplyResult, error) {
	res := ProductApplyResult{ProductSysID: productSysID}
	err := a.tx.InTx(ctx, func(tx ApplyTx) error {
		res = ProductApplyResult{ProductSysID: productSysID}
		code, locked, found, err := tx.LockProduct(ctx, productSysID)
		if err != nil {
			return err
		}
		res.ProductCode = code
		switch {
		case !found:
			res.Skipped, res.SkipReason = true, SkipProductNotFound
			return nil
		case locked:
			res.Skipped, res.SkipReason = true, SkipLocked
			return nil
		}
		plan, err := a.planner.ForReader(tx.Reader()).PlanChunk(ctx, scope, []int64{productSysID})
		if err != nil {
			return fmt.Errorf("re-plan product %d: %w", productSysID, err)
		}
		for _, it := range plan.Items {
			if it.ProductSysID != productSysID || it.SkipReason != "" {
				continue
			}
			if err := applyItem(ctx, tx, it, actor, &res); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ProductApplyResult{}, err
	}
	return res, nil
}

func applyItem(ctx context.Context, tx ApplyTx, it Item, actor string, res *ProductApplyResult) error {
	for _, c := range it.Changes {
		if c.ParamCode == it.TriggerParamCode {
			continue // never modify the trigger row itself
		}
		var err error
		switch {
		case c.Kind == ChangeClear:
			var deleted bool
			deleted, err = tx.DeleteValue(ctx, it.ProductSysID, c.ParamID)
			if deleted {
				res.ValuesCleared++
			}
		case c.NewNumeric != nil:
			err = tx.UpsertNumeric(ctx, it.ProductSysID, c.ParamID, *c.NewNumeric, FilledByLookupSync, actor)
			res.ValuesWritten++
		case c.NewText != nil:
			err = tx.UpsertText(ctx, it.ProductSysID, c.ParamID, *c.NewText, FilledByLookupSync, actor)
			res.ValuesWritten++
		default:
			err = fmt.Errorf("change %s/%s has no value", it.TriggerParamCode, c.ParamCode)
		}
		if err != nil {
			return fmt.Errorf("apply %s on product %d: %w", c.ParamCode, it.ProductSysID, err)
		}
	}
	return nil
}
