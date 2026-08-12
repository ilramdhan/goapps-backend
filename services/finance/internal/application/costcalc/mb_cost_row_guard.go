package costcalc

import (
	"context"
	"errors"
	"fmt"
)

// MBCostRowChecker answers whether a cst_product_cost row belongs to a Master Batch
// product, so the manual verify / approve RPCs can refuse it.
//
// MB rows are owned end-to-end by the MB_BATCH path: mbbatch writes them CALCULATED and
// MB Push-to-Head consumes them, flipping CALCULATED -> APPROVED inside its own
// transaction (CostResultRepository.MarkApprovedFromCalculatedTx) together with the
// cst_mb_cost upsert. Push requires the source row to be exactly CALCULATED. So a user
// hand-verifying an MB row through the generic Cost Results screen does not corrupt a
// number — it silently makes the next push SKIP that MB, which is worse: the MB keeps
// its stale cst_mb_cost value with no error anywhere.
//
// The guard blocks only the two manual RPC handlers. MarkApprovedFromCalculatedTx, the
// legitimate push path, is untouched: it lives on the repository and is called from
// mbpush, never through these handlers.
type MBCostRowChecker interface {
	IsMBCostRow(ctx context.Context, costID int64) (bool, error)
}

// ErrMBCostNotManuallyTransitionable is returned when verify / approve targets a cost row
// belonging to an MB product.
var ErrMBCostNotManuallyTransitionable = errors.New(
	"MB cost results are verified and approved by MB Push to Head, not from the cost results screen")

// rejectMBCostRow fails a manual status transition whose target row is MB-typed. A nil
// checker disables the guard (tests, and any wiring that omits it) — behavior then
// matches the pre-guard code exactly. A checker error is propagated rather than
// swallowed into "not MB", so a database blip cannot let an MB transition through.
func rejectMBCostRow(ctx context.Context, guard MBCostRowChecker, costID int64) error {
	if guard == nil {
		return nil
	}
	isMB, err := guard.IsMBCostRow(ctx, costID)
	if err != nil {
		return fmt.Errorf("check MB cost row: %w", err)
	}
	if isMB {
		return fmt.Errorf("cost %d: %w", costID, ErrMBCostNotManuallyTransitionable)
	}
	return nil
}
