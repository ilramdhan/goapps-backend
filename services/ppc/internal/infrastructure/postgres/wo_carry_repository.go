package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"

	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// prefixedWOColumns renders woColumns qualified by a table alias, so the
// candidate query can join machine and plan_item without ambiguity while
// still feeding woDTO.dest() the exact column order it expects. Deriving it
// from woColumns rather than restating the list keeps the two from drifting
// when a column is added to work_order.
func prefixedWOColumns(alias string) string {
	cols := strings.Split(woColumns, ",")
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		out = append(out, alias+"."+strings.TrimSpace(c))
	}
	return strings.Join(out, ", ")
}

// Ensure WOCarryRepository satisfies the interface.
var _ workorderdomain.CarryCandidateRepository = (*WOCarryRepository)(nil)

// WOCarryRepository implements the WO carry-forward read queries against
// work_order + wo_production_actual + the self-referencing carry linker.
type WOCarryRepository struct{ db *DB }

// NewWOCarryRepository builds a WOCarryRepository.
func NewWOCarryRepository(db *DB) *WOCarryRepository { return &WOCarryRepository{db: db} }

// producedExpr sums the production booked against a WO.
//
// wo_production_actual is an ETL table keyed by (wpa_wo_id, wpa_date,
// wpa_shift) and uq_wpa_wo_date_shift guarantees one row per key, so a plain
// SUM cannot fan out. Per row we prefer wpa_qty_actual — the operator's
// correction — and fall back to the ETL bobbin count; both are NULL on a
// pre-production row.
const producedExpr = `COALESCE((
	SELECT COALESCE(SUM(COALESCE(a.wpa_qty_actual, a.wpa_qty_bobbin, 0)), 0)
	FROM wo_production_actual a
	WHERE a.wpa_wo_id = w.wo_id
), 0)`

// carriedAwayExpr sums every quantity already carried off this WO, across all
// target months rather than just the one being previewed. Scoping this to the
// target month would let Aug->Sep and Aug->Oct each carry the full remainder
// and double-plan the source (the defect found in the plan-item carry path).
//
// wo_ref_wo_id is overloaded — it also links TEMPLATE duplicates and the
// revision chain — so the CONTINUATION filter is what makes this a carry
// debit rather than a count of every WO that merely references this one.
const carriedAwayExpr = `COALESCE((
	SELECT SUM(c.wo_qty_target)
	FROM work_order c
	WHERE c.wo_ref_wo_id = w.wo_id
	AND c.wo_ref_type = 'CONTINUATION'
), 0)`

// carriedMonthsExpr lists the months this WO has already been carried into,
// so the UI can say where the quantity went instead of only that it is gone.
const carriedMonthsExpr = `(
	SELECT COALESCE(ARRAY_AGG(DISTINCT TO_CHAR(c.wo_deadline, 'YYYY-MM')
		ORDER BY TO_CHAR(c.wo_deadline, 'YYYY-MM')), '{}')
	FROM work_order c
	WHERE c.wo_ref_wo_id = w.wo_id
	AND c.wo_ref_type = 'CONTINUATION'
)`

// ListCandidates returns every WO whose deadline falls in sourceMonth — both
// eligible and ineligible — with its production and carry coverage.
//
// Ineligible orders are returned carrying a reason rather than filtered out:
// a planner who cannot find a WO needs to know why it is absent, not be shown
// a shorter list.
func (r *WOCarryRepository) ListCandidates(ctx context.Context, sourceMonth, _ string) ([]*workorderdomain.CarryCandidate, error) {
	query := fmt.Sprintf(`SELECT %s,
	COALESCE(m.machine_no, ''),
	COALESCE(ppi.ppi_cpm_product_sys_id, 0),
	%s, %s, %s
	FROM work_order w
	LEFT JOIN machine m ON m.machine_id = w.wo_machine_id
	LEFT JOIN production_plan_item ppi ON ppi.ppi_id = w.wo_plan_item_id
	WHERE TO_CHAR(w.wo_deadline, 'YYYY-MM') = $1
	ORDER BY w.wo_no`,
		prefixedWOColumns("w"), producedExpr, carriedAwayExpr, carriedMonthsExpr)

	rows, err := r.db.QueryContext(ctx, query, sourceMonth)
	if err != nil {
		return nil, fmt.Errorf("list WO carry candidates: %w", err)
	}
	defer closeRows(rows)

	var candidates []*workorderdomain.CarryCandidate
	for rows.Next() {
		var (
			dto          woDTO
			machineNo    string
			productSysID int64
			c            workorderdomain.CarryCandidate
		)
		dest := dto.dest()
		dest = append(dest, &machineNo, &productSysID,
			&c.Coverage.QtyProduced, &c.Coverage.QtyAlreadyCarried,
			pq.Array(&c.Coverage.CarriedToMonths))
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan WO carry candidate: %w", err)
		}
		wo, err := dto.toEntity()
		if err != nil {
			return nil, fmt.Errorf("rebuild WO carry candidate: %w", err)
		}
		c.WO = wo
		c.MachineLabel = machineNo
		c.ProductSysID = productSysID
		if reason, blocked := woIneligibilityReasons[dto.Status]; blocked {
			c.IneligibilityReason = reason
		} else if c.QtyRemaining() <= 0 {
			c.IneligibilityReason = "production and earlier carries already cover the target quantity"
		}
		candidates = append(candidates, &c)
	}
	return candidates, rows.Err()
}

// woIneligibilityReasons explains, in the planner's language, why a status can
// never be carried. Statuses absent from this map are eligible — SUBMITTED and
// PC_APPROVED deliberately among them, since a WO can fall short of its target
// long before it is fully approved.
var woIneligibilityReasons = map[string]string{
	workorderdomain.StatusDraft:     "is still a draft — confirm it first",
	workorderdomain.StatusRejected:  "was rejected — create a new plan item instead",
	workorderdomain.StatusCompleted: "production is already complete",
	workorderdomain.StatusClosed:    "production is closed and its final quantity is locked",
	workorderdomain.StatusCancelled: "was cancelled — create a new plan item instead",
}

// IsAlreadyCarriedInto reports whether sourceWOID already has a continuation
// landing in targetMonth. This guards the repeat click; the quantity debit in
// carriedAwayExpr is what guards against carrying the same remainder into two
// different months.
func (r *WOCarryRepository) IsAlreadyCarriedInto(ctx context.Context, sourceWOID int64, targetMonth string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM work_order c
			WHERE c.wo_ref_wo_id = $1
			AND c.wo_ref_type = 'CONTINUATION'
			AND TO_CHAR(c.wo_deadline, 'YYYY-MM') = $2
		)`, sourceWOID, targetMonth,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check existing carry for WO: %w", err)
	}
	return exists, nil
}

// CarryCoverage returns the production and carry debit for one WO.
func (r *WOCarryRepository) CarryCoverage(ctx context.Context, woID int64) (workorderdomain.CoverCoverage, error) {
	var c workorderdomain.CoverCoverage
	err := r.db.QueryRowContext(ctx,
		`SELECT
			COALESCE((SELECT COALESCE(SUM(COALESCE(a.wpa_qty_actual, a.wpa_qty_bobbin, 0)), 0)
				FROM wo_production_actual a WHERE a.wpa_wo_id = $1), 0),
			COALESCE((SELECT SUM(c.wo_qty_target)
				FROM work_order c
				WHERE c.wo_ref_wo_id = $1 AND c.wo_ref_type = 'CONTINUATION'), 0)`,
		woID,
	).Scan(&c.QtyProduced, &c.QtyAlreadyCarried)
	if err != nil {
		return c, fmt.Errorf("carry coverage for work order: %w", err)
	}
	return c, nil
}
