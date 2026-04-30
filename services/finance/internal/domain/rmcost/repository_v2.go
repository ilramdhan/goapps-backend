// Package rmcost — V2 repository contracts for cost-detail snapshots and
// inline-edit operations.
package rmcost

import (
	"context"

	"github.com/google/uuid"
)

// CostDetailRepository persists `cst_rm_cost_detail` rows. Implemented in
// infrastructure/postgres alongside the V1 cost repository.
type CostDetailRepository interface {
	// UpsertAll replaces all detail rows for the given cost ID with the
	// supplied set, inside a single transaction. Used after a full V2 calc pass.
	UpsertAll(ctx context.Context, costID uuid.UUID, details []*CostDetail) error

	// GetByID returns one detail row. Returns ErrNotFound when absent.
	GetByID(ctx context.Context, id uuid.UUID) (*CostDetail, error)

	// ListByCostID returns all detail rows for one cost row, ordered by
	// (item_code, grade_code) ASC.
	ListByCostID(ctx context.Context, costID uuid.UUID) ([]*CostDetail, error)

	// UpdateSnapshot writes the per-stage values for one detail row (used
	// after fix_rate edit recompute).
	UpdateSnapshot(ctx context.Context, detail *CostDetail) error

	// DeleteByCostID removes every detail belonging to one cost row.
	DeleteByCostID(ctx context.Context, costID uuid.UUID) error
}

// CostInputsRepository augments the V1 Repository with V2-specific writes.
// We keep V1 Repository unchanged (back-compat) and layer V2 reads/writes here.
type CostInputsRepository interface {
	// UpdateInputs persists the V2 marketing snapshot, simulation rate, and
	// flags onto a cost row. Also updates cost_marketing/cost_simulation if
	// the caller supplies recomputed values.
	UpdateInputs(ctx context.Context, costID uuid.UUID, in V2Inputs, rates V2Rates, costMkt, costSim float64, updatedBy string) error

	// UpdateFLAndCostVal writes a fresh fl_rate and (optionally) a fresh
	// cost_val on the cost row. Called after a per-detail fix_rate edit
	// changes the MAX across the group's details.
	UpdateFLAndCostVal(ctx context.Context, costID uuid.UUID, flRate float64, costVal *float64, updatedBy string) error
}
