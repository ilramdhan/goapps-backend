package erpintegration

// batch_derive.go holds the Derive-step support of the Batch aggregate and
// the transaction-scoped store the step needs (plan-05 P4-T4; design Part 1
// §5.3, Part 2 §6, §9.2).

import (
	"context"
	"fmt"
	"time"
)

// ReopenDerivation moves a DERIVED or VALIDATED batch back to COVERED because
// a re-derive produced blocking rows (V-08 and the other row errors): the
// batch must not stay DERIVED on a derivation that no longer passes (plan-05
// P4-T4 step 4, "stay COVERED with issues"). Coverage and demand are kept
// (they did not change, so V-10 freshness is not reset); the rule snapshot,
// hash, control totals and the warnings ack belong to the discarded
// derivation and are cleared. The step replaces the std rows in the same
// transaction so the failing rows stay visible. A frozen (pushed) batch is
// never reopened.
func (b *Batch) ReopenDerivation(actor string, at time.Time) (Invalidation, error) {
	a, err := normalizeActor(actor)
	if err != nil {
		return Invalidation{}, err
	}
	if b.IsFrozen() {
		return Invalidation{}, ErrBatchFrozen
	}
	if b.status != StatusDerived && b.status != StatusValidated {
		return Invalidation{}, fmt.Errorf("%w: reopen derivation in %s", ErrInvalidTransition, b.status)
	}
	inv := Invalidation{StdRows: true, RuleSnapshot: true, WarningsAck: true}
	b.ruleSnapshot, b.ruleHash, b.totals = nil, "", nil
	b.warningsAck = nil
	b.status = StatusCovered
	b.touch(a, at)
	return inv, nil
}

// DeriveStore is the transaction-scoped store of the Derive step. The PG
// BatchTxRunner hands a BatchStore that also implements DeriveStore; every
// read shares the G11-locked transaction, so the period-lock check, the AX
// inputs and the std-row replacement are consistent with the status update.
type DeriveStore interface {
	BatchStore
	// ListCoverage returns the batch's coverage rows ordered by (item, shade).
	ListCoverage(ctx context.Context) ([]CoverageLine, error)
	// IsPeriodLocked is the G10 check inside the transaction (FOR SHARE, so
	// the answer holds until commit). No lock row means false.
	IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error)
	// LoadAxComponents returns, per cost id, the AX inputs of the ACTUAL
	// APPROVED cst_product_cost row of the period joined with the product's
	// ERP attributes. Ids that are no longer APPROVED for the period (or
	// lack cpc_total_rm_cost) are absent: the row derives as NO_AX.
	LoadAxComponents(ctx context.Context, period string, costIDs []int64) (map[int64]AxComponents, error)
	// ShadeNames returns cost_erp_shade names keyed by the trimmed,
	// upper-cased shade code. Unknown codes are absent.
	ShadeNames(ctx context.Context, shadeCodes []string) (map[string]string, error)
	// ReplaceStdRows deletes the batch's cst_erp_std_cost rows and inserts
	// rows. Save does not delete them again through Invalidation.StdRows.
	ReplaceStdRows(ctx context.Context, period string, rows []StdRow) (int64, error)
}
