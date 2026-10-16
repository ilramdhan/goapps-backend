package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
)

// LookupSyncApplyRepository implements lookupsync.TxRunner: each InTx call
// opens one transaction and hands the applier a tx-bound lookupsync.ApplyTx.
type LookupSyncApplyRepository struct {
	db *DB
}

var _ lookupsync.TxRunner = (*LookupSyncApplyRepository)(nil)

// NewLookupSyncApplyRepository creates the tx runner for lookup-sync apply.
func NewLookupSyncApplyRepository(db *DB) *LookupSyncApplyRepository {
	return &LookupSyncApplyRepository{db: db}
}

// InTx implements lookupsync.TxRunner.
func (r *LookupSyncApplyRepository) InTx(ctx context.Context, fn func(tx lookupsync.ApplyTx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := fn(&lookupSyncApplyTx{tx: tx}); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

type lookupSyncApplyTx struct {
	tx *sql.Tx
}

var _ lookupsync.ApplyTx = (*lookupSyncApplyTx)(nil)

func (t *lookupSyncApplyTx) Reader() lookupsync.SyncReader {
	return (&LookupSyncRepository{}).ForTx(t.tx)
}

func (t *lookupSyncApplyTx) LockProduct(ctx context.Context, productSysID int64) (string, bool, bool, error) {
	const q = `SELECT cpm_product_code, cpm_is_locked FROM cost_product_master WHERE cpm_product_sys_id = $1 FOR UPDATE`
	var (
		code   string
		locked bool
	)
	err := t.tx.QueryRowContext(ctx, q, productSysID).Scan(&code, &locked)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, fmt.Errorf("lock product %d: %w", productSysID, err)
	}
	return code, locked, true, nil
}

func (t *lookupSyncApplyTx) UpsertNumeric(ctx context.Context, productSysID int64, paramID uuid.UUID, v decimal.Decimal, filledBy, actor string) error {
	s := v.String()
	return upsertValueTx(ctx, t.tx, productSysID, paramID, &s, nil, nil, filledBy, actor)
}

func (t *lookupSyncApplyTx) UpsertText(ctx context.Context, productSysID int64, paramID uuid.UUID, v, filledBy, actor string) error {
	return upsertValueTx(ctx, t.tx, productSysID, paramID, nil, &v, nil, filledBy, actor)
}

func (t *lookupSyncApplyTx) DeleteValue(ctx context.Context, productSysID int64, paramID uuid.UUID) (bool, error) {
	const q = `DELETE FROM cost_product_parameter WHERE cpp_product_sys_id = $1 AND cpp_param_id = $2`
	res, err := t.tx.ExecContext(ctx, q, productSysID, paramID)
	if err != nil {
		return false, fmt.Errorf("delete cpp value %s: %w", paramID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete cpp value %s rows: %w", paramID, err)
	}
	return n > 0, nil
}
