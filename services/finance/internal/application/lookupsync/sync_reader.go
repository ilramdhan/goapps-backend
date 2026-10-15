package lookupsync

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// MasterRef points at one lookup master record: the master code plus the key
// products store for it. For MB_SPIN the key is mst_mb_spin.mbs_id.
type MasterRef struct {
	LookupMasterCode string
	MasterKey        string
}

// ProductFilter mirrors the Product Master list filter
// (costproductmaster.ListFilter: Search / ProductTypeIDs / ShadeCode / ActiveFilter).
type ProductFilter struct {
	Search         string
	ProductTypeIDs []int32
	ShadeCode      string
	ActiveFilter   string // "" | "all" | "active" | "inactive"
}

// Scope selects the (product, trigger) pairs a plan covers.
//
// MasterRef and Filter are mutually exclusive; when both are nil the scope is
// "every trigger on ProductSysIDs". ProductSysIDs combined with MasterRef or
// Filter is an INTERSECTION (AND) — this is how the worker re-plans one chunk
// of a master_ref / filter job without changing the query shape.
type Scope struct {
	MasterRef     *MasterRef
	Filter        *ProductFilter
	ProductSysIDs []int64
}

// TriggerRow is one (product, MASTER_LOOKUP trigger param) pair in scope.
type TriggerRow struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	// IsLocked is cost_product_master.cpm_is_locked.
	IsLocked         bool
	TriggerParamID   uuid.UUID
	TriggerParamCode string
	// LookupMasterCode is the trigger's mst_parameter.lookup_master_code.
	LookupMasterCode string
	// StoredKey is the trigger's cpp_value_text ("" when no row / empty).
	StoredKey string
	// MBSpinID is cpp_value_mb_spin_id (MB_SPIN triggers only; nil otherwise).
	MBSpinID *uuid.UUID
}

// ProductParamKey addresses one cost_product_parameter cell.
type ProductParamKey struct {
	ProductSysID int64
	ParamID      uuid.UUID
}

// StoredValue is the current cost_product_parameter row of a child. Exactly one
// field is non-nil (cpp_one_value_chk).
type StoredValue struct {
	Numeric *decimal.Decimal
	Text    *string
	Flag    *bool
}

// ChildState is the state of one applicable child cell of a product.
type ChildState struct {
	// Stored is nil when the product has no cost_product_parameter row for the child.
	Stored *StoredValue
}

// SyncReader is the read port the Planner needs. P4 implements it in postgres;
// the worker (P6) passes a tx-scoped implementation so its re-plan sees (and
// can lock) the rows it is about to write.
type SyncReader interface {
	// ListTriggerRows returns the trigger rows of the next up-to-limit DISTINCT
	// products in scope with cpm_sys_id > afterProductSysID, ordered by
	// (product_sys_id, trigger_param_code). All triggers of a returned product
	// must be in the same page (keyset paging by product). Fewer than limit
	// distinct products means the scope is exhausted.
	//
	// Only triggers whose lookup_master_code is one of SyncableMasters and that
	// are applicable to the product (cost_product_applicable_param) are
	// returned. For Scope.MasterRef, only triggers of that master whose stored
	// value points at MasterKey; for MB_SPIN that is
	// cpp_value_mb_spin_id = key OR cpp_value_text IN (spin's orion code, key).
	ListTriggerRows(ctx context.Context, scope Scope, afterProductSysID int64, limit int) ([]TriggerRow, error)

	// LoadChildStates returns, for every (product, param) pair among the inputs
	// that is APPLICABLE to the product (cost_product_applicable_param), its
	// current value. Pairs absent from the map are not applicable and are never
	// written by the sync (the sync never adds applicability).
	LoadChildStates(ctx context.Context, productSysIDs []int64, paramIDs []uuid.UUID) (map[ProductParamKey]ChildState, error)

	// CountMBSpinsByOrionCode counts non-deleted mst_mb_spin rows whose
	// mbs_orion_item_code = code (same rule as mbspin.Repository
	// ResolveUniqueByOrionItemCode). > 1 means a text-only reference is ambiguous.
	CountMBSpinsByOrionCode(ctx context.Context, code string) (int, error)

	// ProductGradeCodeByName maps a legacy product-grade reference stored as the
	// grade NAME to its pg_code (non-deleted rows). ok=false when no grade (or
	// more than one) has that name.
	ProductGradeCodeByName(ctx context.Context, name string) (code string, ok bool, err error)
}

// FillResolver is the slice of *Resolver the Planner uses.
type FillResolver interface {
	Resolve(ctx context.Context, lookupMasterCode, selectedKey, sourceParamCode string) (FillResult, error)
}

// FillGroupLister lists the fill-group children of a trigger param.
// parameter.Repository satisfies it (GetByFillGroup).
type FillGroupLister interface {
	GetByFillGroup(ctx context.Context, fillGroupCode string) ([]*parameter.Parameter, error)
}

var (
	_ FillResolver    = (*Resolver)(nil)
	_ FillGroupLister = (parameter.Repository)(nil)
)
