package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// Plan-item carry-forward queries.
//
// The one non-obvious piece here is coverage. A plan item's qty_target is the
// planner's claim against its demand; every work order raised against it
// consumes part of that claim through wo_plan_item_link.wpl_qty_contribution.
// Carrying the whole qty_target of an item that already has work orders would
// re-book production already committed downstream of the same demand — which is
// exactly the double-count S-2.2 forbids. So every carry is sized against
// qty_target minus non-void coverage, never against qty_target itself.
//
// Cancelled and rejected work orders are excluded from coverage
// (workorder.VoidStatuses): they produced nothing, and counting them would
// permanently strand the quantity they had claimed.

// carryEligibleStatuses are the plan-item statuses whose work has not started
// and is not finished — the only ones the carry-forward flow offers. Sourced
// from the domain so the SQL and planitem.IsCarryCandidate cannot drift.
var carryEligibleStatuses = []string{planitem.StatusDraft, planitem.StatusConfirmed}

// coverageSubquery sums the non-void work-order contributions for one plan
// item, correlated on the alias given. COALESCE keeps an item with no work
// orders at zero rather than NULL.
func coverageSubquery(alias string, voidStart int) (covered, count string, args []interface{}) {
	placeholders := make([]string, 0, len(workorder.VoidStatuses))
	for i, s := range workorder.VoidStatuses {
		placeholders = append(placeholders, fmt.Sprintf("$%d", voidStart+i))
		args = append(args, s)
	}
	notVoid := "wo.wo_status NOT IN (" + strings.Join(placeholders, ", ") + ")"
	join := `FROM wo_plan_item_link wpl
			JOIN work_order wo ON wo.wo_id = wpl.wpl_wo_id
			WHERE wpl.wpl_plan_item_id = ` + alias + ` AND ` + notVoid

	covered = `COALESCE((SELECT SUM(wpl.wpl_qty_contribution) ` + join + `), 0)`
	count = `COALESCE((SELECT COUNT(*) ` + join + `), 0)`
	return covered, count, args
}

// carriedAwaySubqueries report what the aliased row has already handed to
// carried children, from both angles the domain needs.
//
// Deliberately NOT scoped to one target month. A month-scoped debit would let
// Aug→Sep and then Aug→Oct each carry the full uncovered quantity, planning it
// twice — the double-count S-2.2 forbids. The quantity debit is therefore
// global, and only the duplicate-month guard is per-target.
//
// qtyAway  — SUM(qty_target) over every child anywhere naming this row.
// months   — the distinct months carried into, ascending, as a text array.
func carriedAwaySubqueries(alias string) (qtyAway, months string) {
	from := `FROM production_plan_item c WHERE c.ppi_carry_from_item_id = ` + alias
	qtyAway = `COALESCE((SELECT SUM(c.ppi_qty_target) ` + from + `), 0)`
	months = `COALESCE((SELECT ARRAY_AGG(DISTINCT c.ppi_month ORDER BY c.ppi_month) ` + from + `), '{}')`
	return qtyAway, months
}

// demandLabelOfPlanItem renders, in words, the demand an item ultimately
// serves: its own for an FG item, or its parent's for a cascade INTERMEDIATE
// item. Contract number first, then the demand type, so the planner sees
// something recognizable — never a raw demand id. Only one level up is walked;
// a deeper chain yields ” and the UI says the source is a parent plan item.
const demandLabelOfPlanItem = `COALESCE((
		SELECT NULLIF(TRIM(COALESCE(d.pd_contract_no, '')), '')
		FROM production_demand d
		WHERE d.pd_id = COALESCE(ppi.ppi_demand_id, (
			SELECT p.ppi_demand_id FROM production_plan_item p
			WHERE p.ppi_id = ppi.ppi_parent_item_id))), '')`

// ListCarryCandidates returns the plan items in sourceMonth whose status still
// permits carrying, each decorated with its work-order coverage and whether
// targetMonth already holds a row carried from it.
//
// An item whose quantity is fully covered by work orders is still returned: the
// UI needs to say why it cannot be carried rather than silently omitting it
// (the same principle S-2.3 states for work orders).
func (r *PlanItemRepository) ListCarryCandidates(
	ctx context.Context, sourceMonth, targetMonth string,
) ([]*planitem.CarryCandidate, error) {
	args := []interface{}{sourceMonth, targetMonth}

	statusPlaceholders := make([]string, 0, len(carryEligibleStatuses))
	for i, s := range carryEligibleStatuses {
		statusPlaceholders = append(statusPlaceholders, fmt.Sprintf("$%d", 3+i))
		args = append(args, s)
	}

	covered, count, voidArgs := coverageSubquery("ppi.ppi_id", 3+len(carryEligibleStatuses))
	args = append(args, voidArgs...)
	qtyAway, months := carriedAwaySubqueries("ppi.ppi_id")

	query := `SELECT ` + planItemColumnsQualified + `,
			` + covered + `, ` + count + `, ` + qtyAway + `, ` + months + `,
			` + demandLabelOfPlanItem + `
		FROM production_plan_item ppi
		WHERE ppi.ppi_month = $1
		  AND ppi.ppi_status IN (` + strings.Join(statusPlaceholders, ", ") + `)
		ORDER BY ppi.ppi_deadline ASC, ppi.ppi_sequence ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list plan carry-forward candidates: %w", err)
	}
	defer closeRows(rows)

	var result []*planitem.CarryCandidate
	for rows.Next() {
		var (
			dto            planItemDTO
			qtyCovered     float64
			woCount        int32
			qtyCarriedAway float64
			carriedMonths  pq.StringArray
			demandLabel    string
		)
		dest := append(dto.dest(), &qtyCovered, &woCount, &qtyCarriedAway, &carriedMonths, &demandLabel)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan plan carry candidate: %w", err)
		}
		result = append(result, &planitem.CarryCandidate{
			Item: dto.toEntity(),
			Coverage: planitem.Coverage{
				QtyCovered:      qtyCovered,
				WorkOrderCount:  woCount,
				QtyCarriedAway:  qtyCarriedAway,
				CarriedToMonths: carriedMonths,
			},
			DemandLabel: demandLabel,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plan carry candidates: %w", err)
	}
	return result, nil
}

// CarryCoverage answers the coverage and duplicate questions for one plan item,
// so a carry can be validated server-side rather than trusting the numbers the
// client was shown.
//
// targetMonth is accepted for interface symmetry with ListCarryCandidates but
// is not part of the query: both debits are deliberately source-scoped, and the
// per-target duplicate check is the application layer's (see carryIntoMonth,
// which tests CarriedToMonths against the requested month).
func (r *PlanItemRepository) CarryCoverage(
	ctx context.Context, planItemID int64, _ string,
) (planitem.Coverage, error) {
	args := []interface{}{planItemID}
	covered, count, voidArgs := coverageSubquery("$1", 2)
	args = append(args, voidArgs...)
	qtyAway, months := carriedAwaySubqueries("$1")

	query := `SELECT ` + covered + `, ` + count + `, ` + qtyAway + `, ` + months

	var c planitem.Coverage
	var carriedMonths pq.StringArray
	err := r.db.QueryRowContext(ctx, query, args...).
		Scan(&c.QtyCovered, &c.WorkOrderCount, &c.QtyCarriedAway, &carriedMonths)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c, planitem.ErrNotFound
		}
		return c, fmt.Errorf("failed to read plan item carry coverage: %w", err)
	}
	c.CarriedToMonths = carriedMonths
	return c, nil
}
