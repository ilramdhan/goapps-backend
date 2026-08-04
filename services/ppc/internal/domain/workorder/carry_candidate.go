package workorder

import "context"

// CoverCoverage holds the production-actual-derived figures for one WO.
// These numbers come from wo_production_actual and are the sole source of
// truth for "what was produced" — QtyTarget − QtyProduced = the remaining
// qty the carry would pass to a continuation.
type CoverCoverage struct {
	// QtyProduced is the total produced quantity across all production-actual
	// rows for this WO. Per row: COALESCE(wpa_qty_actual, wpa_qty_bobbin, 0) —
	// the operator's adjusted figure takes precedence over the ETL bobbin count.
	// When no production has started yet, this is 0: QtyRemaining = QtyTarget.
	QtyProduced float64
	// QtyAlreadyCarried is SUM(qty_target) over every continuation WO that
	// names this one as its source, regardless of target month. This is the
	// source-scoped debit: a WO whose remaining qty has already been carried
	// into one month cannot have the same qty carried into another.
	QtyAlreadyCarried float64
	// CarriedToMonths lists the distinct months this WO has been carried into,
	// ascending. A duplicate-per-month guard scoped to one target (S-2.4:
	// carrying twice must not duplicate the same source→target pair).
	CarriedToMonths []string
}

// IsAlreadyCarriedInto reports whether this WO has already been carried into
// at least one month.
func (c CoverCoverage) IsAlreadyCarriedInto() bool { return len(c.CarriedToMonths) > 0 }

// CarryCandidate is one WO considered at month start, decorated with the
// production and carry figures the planner needs to decide.
type CarryCandidate struct {
	WO       *WorkOrder
	Coverage CoverCoverage
	// MachineLabel is the human-readable machine identifier (machine_no), never
	// a raw id.
	MachineLabel string
	// ProductLabel names the product this WO produces, or "" when unmapped.
	// Resolved in the application layer from ProductSysID, since the product
	// master lives in finance rather than this service's database.
	ProductLabel string
	// ProductSysID is the finance product this WO's plan item points at, or 0
	// when the plan item is missing or unmapped. Never surfaced to the user —
	// it exists only so the application layer can resolve ProductLabel.
	ProductSysID int64
	// IneligibilityReason is set when the WO may not be carried. Empty when it
	// is eligible. Ineligible candidates are listed below eligible ones, so a
	// full month still tells the planner what happened to every WO.
	IneligibilityReason string
}

// QtyRemaining is the quantity of the WO's target not yet produced: what
// CARRY_AS_IS carries. Both debits are applied — what was already produced,
// and what was already handed to a continuation in ANY month.
func (c *CarryCandidate) QtyRemaining() float64 {
	remaining := c.WO.QtyTarget() - c.Coverage.QtyProduced - c.Coverage.QtyAlreadyCarried
	if remaining < 0 {
		return 0
	}
	return remaining
}

// CarryCandidateRepository defines read-only queries for WO carry-forward
// candidates. Separated from Repository because these queries cross
// production_actual (an ETL table) and the carry linker query, and they return
// the read-model CarryCandidate rather than the full WorkOrder aggregate.
type CarryCandidateRepository interface {
	// ListCandidates returns every WO in sourceMonth — both eligible and
	// ineligible — with its production and carry coverage. The caller owns
	// further decoration (machine label, product label).
	ListCandidates(ctx context.Context, sourceMonth, targetMonth string) ([]*CarryCandidate, error)
	// IsAlreadyCarriedInto reports whether sourceWOID has already been carried
	// into targetMonth. This is the narrower per-target duplicate guard; the
	// qty-level source-scoped guard lives in QtyAlreadyCarried.
	IsAlreadyCarriedInto(ctx context.Context, sourceWOID int64, targetMonth string) (bool, error)
	// CarryCoverage returns the production and carry debit for one WO so the
	// handler can re-verify the eligibility numbers the client saw.
	CarryCoverage(ctx context.Context, woID int64) (CoverCoverage, error)
}
