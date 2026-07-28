package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dashboard"
)

// DashboardRepository gathers read-only aggregates for the daily-performance and
// morning-review dashboards. All figures are computed best-effort from live
// planning + daily-perf data; areaCode "" spans every area.
type DashboardRepository struct {
	db *DB
}

// NewDashboardRepository creates a new DashboardRepository.
func NewDashboardRepository(db *DB) *DashboardRepository {
	return &DashboardRepository{db: db}
}

// monthStart returns the first day of date's month (UTC midnight).
func monthStart(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// areaCond appends an optional area filter to a WHERE-ready query using the given
// column, returning the extended arg slice. Empty area adds no filter.
func areaCond(query *string, col, area string, args []any) []any {
	if area == "" {
		return args
	}
	*query += fmt.Sprintf(" AND %s = $%d", col, len(args)+1)
	return append(args, area)
}

// AreaAggregate loads the daily-performance area aggregate for date + month, for
// the requested Excluding/Including variant, from AREA_DAY efficiency snapshots
// with the downtime (idle positions) and area-shift-log (OT hours) side sums.
func (r *DashboardRepository) AreaAggregate(ctx context.Context, area string, date time.Time, excluding bool) (dashboard.AreaAggregate, error) {
	var agg dashboard.AreaAggregate
	mStart := monthStart(date)

	if err := r.foldSnapshotSums(ctx, area, date, mStart, excluding, &agg); err != nil {
		return agg, err
	}
	if err := r.foldIdlePositions(ctx, area, date, mStart, &agg); err != nil {
		return agg, err
	}
	if err := r.foldOTHours(ctx, area, date, mStart, &agg); err != nil {
		return agg, err
	}
	return agg, nil
}

// foldSnapshotSums sums actual/theoretical/waste from AREA_DAY snapshots for the
// day and the month-to-date window.
func (r *DashboardRepository) foldSnapshotSums(ctx context.Context, area string, date, mStart time.Time, excluding bool, agg *dashboard.AreaAggregate) error {
	query := `
		SELECT
			COALESCE(SUM(es_qty_actual)          FILTER (WHERE es_date = $1), 0),
			COALESCE(SUM(es_qty_theoretical_100) FILTER (WHERE es_date = $1), 0),
			COALESCE(SUM(es_qty_waste)           FILTER (WHERE es_date = $1), 0),
			COALESCE(SUM(es_qty_actual),          0),
			COALESCE(SUM(es_qty_theoretical_100), 0),
			COALESCE(SUM(es_qty_waste),           0)
		FROM efficiency_snapshot
		WHERE es_scope = 'AREA_DAY' AND es_is_excluding = $2
			AND es_date >= $3 AND es_date <= $1`
	args := []any{date, excluding, mStart}
	args = areaCond(&query, "es_area", area, args)
	row := r.db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(
		&agg.ActualToday, &agg.TheoToday, &agg.WasteToday,
		&agg.ActualMTD, &agg.TheoMTD, &agg.WasteMTD,
	); err != nil {
		return fmt.Errorf("dashboard snapshot sums: %w", err)
	}
	return nil
}

// foldIdlePositions counts distinct idle POSITION downtime events for the day and
// the month-to-date window, joined to the machine for the area filter.
func (r *DashboardRepository) foldIdlePositions(ctx context.Context, area string, date, mStart time.Time, agg *dashboard.AreaAggregate) error {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE de.de_date = $1),
			COUNT(*)
		FROM downtime_event de
		JOIN machine m ON m.machine_id = de.de_machine_id
		WHERE de.de_loss_type = 'POSITION'
			AND de.de_date >= $2 AND de.de_date <= $1`
	args := []any{date, mStart}
	args = areaCond(&query, "m.machine_area", area, args)
	var today, mtd int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&today, &mtd); err != nil {
		return fmt.Errorf("dashboard idle positions: %w", err)
	}
	agg.IdleToday = int(today)
	agg.IdleMTD = int(mtd)
	return nil
}

// foldOTHours sums overtime hours from the area shift log for the day and the
// month-to-date window.
func (r *DashboardRepository) foldOTHours(ctx context.Context, area string, date, mStart time.Time, agg *dashboard.AreaAggregate) error {
	query := `
		SELECT
			COALESCE(SUM(asl_ot_hours) FILTER (WHERE asl_date = $1), 0),
			COALESCE(SUM(asl_ot_hours), 0)
		FROM area_shift_log
		WHERE asl_date >= $2 AND asl_date <= $1`
	args := []any{date, mStart}
	args = areaCond(&query, "asl_area", area, args)
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&agg.OTToday, &agg.OTMTD); err != nil {
		return fmt.Errorf("dashboard OT hours: %w", err)
	}
	return nil
}

// McEffGrid loads the machine×shift efficiency cells for the month of date, from
// MACHINE_SHIFT Including snapshots. Cells are ordered machine, date, shift.
func (r *DashboardRepository) McEffGrid(ctx context.Context, area string, date time.Time) ([]dashboard.McEffCell, error) {
	mStart := monthStart(date)
	query := `
		SELECT es.es_machine_id, COALESCE(m.machine_no, ''), es.es_date,
			COALESCE(es.es_shift, ''), COALESCE(es.es_eff_production_pct, 0)
		FROM efficiency_snapshot es
		LEFT JOIN machine m ON m.machine_id = es.es_machine_id
		WHERE es.es_scope = 'MACHINE_SHIFT' AND es.es_is_excluding = FALSE
			AND es.es_date >= $1 AND es.es_date <= $2`
	args := []any{mStart, date}
	args = areaCond(&query, "es.es_area", area, args)
	query += ` ORDER BY es.es_machine_id, es.es_date, es.es_shift`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard mc-eff grid: %w", err)
	}
	defer closeRows(rows)

	var cells []dashboard.McEffCell
	for rows.Next() {
		var (
			c       dashboard.McEffCell
			machine sql.NullInt64
		)
		if err := rows.Scan(&machine, &c.MachineNo, &c.Date, &c.Shift, &c.EffPct); err != nil {
			return nil, fmt.Errorf("scan mc-eff cell: %w", err)
		}
		c.MachineID = machine.Int64
		cells = append(cells, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mc-eff cells: %w", err)
	}
	return cells, nil
}

// MachineActualVsPlan returns the per-machine actual-vs-target rows for a date:
// summed WO target and actual production per running machine, with a changeover
// flag. area "" spans all areas.
func (r *DashboardRepository) MachineActualVsPlan(ctx context.Context, area string, date time.Time) ([]dashboard.MachineRow, error) {
	query := `
		SELECT wo.wo_machine_id, COALESCE(m.machine_no, ''), wo.wo_area,
			COALESCE(SUM(wo.wo_qty_target), 0) AS target,
			COALESCE(SUM(prod.qty_actual), 0)  AS actual,
			BOOL_OR(co.ce_id IS NOT NULL)      AS changeover
		FROM work_order wo
		JOIN machine m ON m.machine_id = wo.wo_machine_id
		LEFT JOIN (
			SELECT wpa_wo_id, SUM(COALESCE(wpa_qty_actual, 0)) AS qty_actual
			FROM wo_production_actual WHERE wpa_date = $1 GROUP BY wpa_wo_id
		) prod ON prod.wpa_wo_id = wo.wo_id
		LEFT JOIN changeover_event co ON co.ce_machine_id = wo.wo_machine_id
			AND co.ce_started_at::date = $1
		WHERE wo.wo_status IN ('RUNNING', 'CHANGEOVER', 'SCHEDULED', 'COMPLETED')`
	args := []any{date}
	args = areaCond(&query, "wo.wo_area", area, args)
	query += ` GROUP BY wo.wo_machine_id, m.machine_no, wo.wo_area ORDER BY m.machine_no`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard actual-vs-plan: %w", err)
	}
	defer closeRows(rows)

	var out []dashboard.MachineRow
	for rows.Next() {
		var mr dashboard.MachineRow
		if err := rows.Scan(&mr.MachineID, &mr.MachineNo, &mr.Area, &mr.QtyTarget, &mr.QtyActual, &mr.IsChangeover); err != nil {
			return nil, fmt.Errorf("scan actual-vs-plan row: %w", err)
		}
		out = append(out, mr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate actual-vs-plan rows: %w", err)
	}
	return out, nil
}

// PendingApprovalWOs returns work orders awaiting approval (SUBMITTED or
// PC_APPROVED), oldest first, with the last-update time used to grade
// auto-approve urgency. area "" spans all areas.
func (r *DashboardRepository) PendingApprovalWOs(ctx context.Context, area string) ([]dashboard.PendingWO, error) {
	query := `
		SELECT wo_id, wo_no, wo_status, wo_updated_at
		FROM work_order
		WHERE wo_status IN ('SUBMITTED', 'PC_APPROVED')
			AND wo_auto_approve_disabled = FALSE`
	args := []any{}
	args = areaCond(&query, "wo_area", area, args)
	query += ` ORDER BY wo_updated_at ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard pending approvals: %w", err)
	}
	defer closeRows(rows)

	var out []dashboard.PendingWO
	for rows.Next() {
		var w dashboard.PendingWO
		if err := rows.Scan(&w.WoID, &w.WoNo, &w.Status, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan pending approval: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending approvals: %w", err)
	}
	return out, nil
}

// PrioritiesDueToday returns work orders whose deadline is on or before date and
// that are not yet closed/completed, sorted by deadline then WO number. area ""
// spans all areas.
func (r *DashboardRepository) PrioritiesDueToday(ctx context.Context, area string, date time.Time) ([]dashboard.Priority, error) {
	query := `
		SELECT wo.wo_id, wo.wo_no, COALESCE(m.machine_no, ''),
			wo.wo_deadline, COALESCE(wo.wo_qty_target, 0)
		FROM work_order wo
		LEFT JOIN machine m ON m.machine_id = wo.wo_machine_id
		WHERE wo.wo_deadline <= $1
			AND wo.wo_status NOT IN ('CLOSED', 'COMPLETED', 'CANCELLED', 'REJECTED')`
	args := []any{date}
	args = areaCond(&query, "wo.wo_area", area, args)
	query += ` ORDER BY wo.wo_deadline ASC, wo.wo_no ASC LIMIT 50`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard priorities: %w", err)
	}
	defer closeRows(rows)

	var out []dashboard.Priority
	for rows.Next() {
		var p dashboard.Priority
		if err := rows.Scan(&p.WoID, &p.WoNo, &p.MachineNo, &p.Deadline, &p.QtyTarget); err != nil {
			return nil, fmt.Errorf("scan priority: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate priorities: %w", err)
	}
	return out, nil
}

// QuickStats returns the morning-review headline counters for area (or all).
func (r *DashboardRepository) QuickStats(ctx context.Context, area string) (dashboard.QuickStats, error) {
	var s dashboard.QuickStats
	if err := r.foldMachineCounts(ctx, area, &s); err != nil {
		return s, err
	}
	if err := r.foldPendingCount(ctx, area, &s); err != nil {
		return s, err
	}
	if err := r.foldUnmatchedSO(ctx, &s); err != nil {
		return s, err
	}
	return s, nil
}

// foldMachineCounts fills running + total active machine counts.
func (r *DashboardRepository) foldMachineCounts(ctx context.Context, area string, s *dashboard.QuickStats) error {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE machine_id IN (
				SELECT wo_machine_id FROM work_order WHERE wo_status = 'RUNNING')),
			COUNT(*)
		FROM machine
		WHERE machine_is_active = TRUE`
	args := []any{}
	args = areaCond(&query, "machine_area", area, args)
	var running, total int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&running, &total); err != nil {
		return fmt.Errorf("dashboard machine counts: %w", err)
	}
	s.MachinesRunning = safeInt64ToInt32(running)
	s.MachinesTotal = safeInt64ToInt32(total)
	return nil
}

// foldPendingCount fills the pending-approval WO count.
func (r *DashboardRepository) foldPendingCount(ctx context.Context, area string, s *dashboard.QuickStats) error {
	query := `SELECT COUNT(*) FROM work_order WHERE wo_status IN ('SUBMITTED', 'PC_APPROVED')`
	args := []any{}
	args = areaCond(&query, "wo_area", area, args)
	var n int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return fmt.Errorf("dashboard pending count: %w", err)
	}
	s.WOsPendingApproval = safeInt64ToInt32(n)
	return nil
}

// foldUnmatchedSO fills the count of staged sales orders not yet pulled to a
// demand (area-agnostic — staging has no area dimension).
func (r *DashboardRepository) foldUnmatchedSO(ctx context.Context, s *dashboard.QuickStats) error {
	const query = `SELECT COUNT(*) FROM sales_order_staging WHERE sos_pulled_to_demand_id IS NULL`
	var n int64
	if err := r.db.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return fmt.Errorf("dashboard unmatched SO: %w", err)
	}
	s.UnmatchedSOCount = safeInt64ToInt32(n)
	return nil
}
