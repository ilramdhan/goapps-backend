package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hrot"
)

// replaceOTFacts recomputes the R1 facts (HR_OT_HOURS, HR_OT_DQ, HR_OT_PARETO, HR_OT_EMP_COUNT, HR_OT_DQ_EMP) of every payroll
// period touched by [d1, d2] from staging, inside the caller's transaction (HD-03/HD-04): facts of
// those periods are deleted and re-inserted, so rows removed at the source disappear from the facts.
// Department = current dept from bi_hr_employee (K15); older facts are restated to it (HD-15).
// The R2 weekly facts (H012) are recomputed in the same transaction.
func replaceOTFacts(ctx context.Context, tx *sql.Tx, d1, d2 time.Time, sourceID uuid.UUID) error {
	from, to := hrot.PayrollPeriodStart(d1), hrot.PayrollPeriodEnd(d2)

	rows, err := loadOTStaging(ctx, tx, from, to)
	if err != nil {
		return err
	}
	dept, err := loadCurrentDept(ctx, tx)
	if err != nil {
		return err
	}
	var loadedFrom time.Time
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(work_date), $1) FROM bi_hr_stg_ot_daily`, from).
		Scan(&loadedFrom); err != nil {
		return fmt.Errorf("read staging start: %w", err)
	}
	loadedFrom = time.Date(loadedFrom.Year(), loadedFrom.Month(), loadedFrom.Day(), 0, 0, 0, 0, time.UTC)

	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_fact_metric
WHERE type IN ($1, $2, $3, $9, $10) AND scenario = 'ACTUAL'
  AND ((periode_grain = $4 AND periode_date BETWEEN $6 AND $7)
    OR (periode_grain = $5 AND periode_date BETWEEN $8 AND $7))`,
		hrot.TypeHours, hrot.TypeDQ, hrot.TypePareto, hrot.GrainDaily, hrot.GrainMonthly,
		from, to, hrot.PayrollPeriodEnd(d1), hrot.TypeEmpCount, hrot.TypeDQEmp); err != nil {
		return fmt.Errorf("delete OT facts: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3, periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, metric_name, metric_category, agg_method,
    source_id, dimension_key, is_active
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$8::numeric,$9,'ACTUAL','VALUE',$10,$11,$12,'',TRUE)`)
	if err != nil {
		return fmt.Errorf("prepare OT fact insert: %w", err)
	}
	defer func() {
		if cErr := stmt.Close(); cErr != nil {
			_ = cErr
		}
	}()

	for _, f := range hrot.Aggregate(rows, dept, from, to, loadedFrom) {
		uom, category, agg := hrFactUnit(f.Type)
		if _, err = stmt.ExecContext(ctx, f.Type, f.Group1, biNullableString(f.Group2), biNullableString(f.Group3),
			f.Grain, f.PeriodDate, f.PeriodLabel, f.Value.String(), uom, category, agg, sourceID); err != nil {
			return fmt.Errorf("insert OT fact: %w", err)
		}
	}
	if err = replaceOTWeekFacts(ctx, tx, d1, d2, sourceID, dept, loadedFrom); err != nil {
		return err
	}
	if err = restateOTDept(ctx, tx); err != nil {
		return err
	}
	return rebuildOTOverMonthFacts(ctx, tx, sourceID)
}

// perEmployeeTypes are the OT fact types keyed by NIK in group_3 (R1 and R2), restated by HD-15.
var perEmployeeTypes = append([]string{hrot.TypeHours, hrot.TypeDQ, hrot.TypeWeekDaily}, r2WeeklyTypes...)

// restateOTDept moves per-employee facts outside the recomputed window to the employee's current
// dept (K15, HD-15), so one employee never splits across two depts in a trend. A NIK has a single
// dept per period, so the update cannot collide on the business key.
func restateOTDept(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `UPDATE bi_fact_metric f
SET group_1 = LEFT(e.dept_name, 120), loaded_at = NOW()
FROM bi_hr_employee e
WHERE f.type = ANY($1) AND f.group_3 = e.nik
  AND e.dept_name IS NOT NULL AND e.dept_name <> ''
  AND f.group_1 <> LEFT(e.dept_name, 120)`, pq.Array(perEmployeeTypes)); err != nil {
		return fmt.Errorf("restate OT dept: %w", err)
	}
	return nil
}

func loadOTStaging(ctx context.Context, tx *sql.Tx, from, to time.Time) ([]hrot.Row, error) {
	rs, err := tx.QueryContext(ctx, `SELECT work_date, nik, COALESCE(dept_name, ''), ot_type,
    ot_tier_1::text, ot_tier_2::text, ot_tier_3::text, ot_calc_hours::text
FROM bi_hr_stg_ot_daily WHERE work_date BETWEEN $1 AND $2`, from, to)
	if err != nil {
		return nil, fmt.Errorf("read OT staging: %w", err)
	}
	defer func() {
		if cErr := rs.Close(); cErr != nil {
			_ = cErr
		}
	}()

	var out []hrot.Row
	for rs.Next() {
		var r hrot.Row
		var t1, t2, t3, calc string
		if err = rs.Scan(&r.WorkDate, &r.NIK, &r.StagingDept, &r.OTType, &t1, &t2, &t3, &calc); err != nil {
			return nil, fmt.Errorf("scan OT staging: %w", err)
		}
		if r.Tier1, err = decimal.NewFromString(t1); err != nil {
			return nil, fmt.Errorf("parse tier 1: %w", err)
		}
		if r.Tier2, err = decimal.NewFromString(t2); err != nil {
			return nil, fmt.Errorf("parse tier 2: %w", err)
		}
		if r.Tier3, err = decimal.NewFromString(t3); err != nil {
			return nil, fmt.Errorf("parse tier 3: %w", err)
		}
		if r.CalcHours, err = decimal.NewFromString(calc); err != nil {
			return nil, fmt.Errorf("parse calc hours: %w", err)
		}
		r.WorkDate = time.Date(r.WorkDate.Year(), r.WorkDate.Month(), r.WorkDate.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, r)
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate OT staging: %w", err)
	}
	return out, nil
}

func loadCurrentDept(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rs, err := tx.QueryContext(ctx, `SELECT nik, dept_name FROM bi_hr_employee WHERE dept_name IS NOT NULL AND dept_name <> ''`)
	if err != nil {
		return nil, fmt.Errorf("read employee dept: %w", err)
	}
	defer func() {
		if cErr := rs.Close(); cErr != nil {
			_ = cErr
		}
	}()

	out := map[string]string{}
	for rs.Next() {
		var nik, dept string
		if err = rs.Scan(&nik, &dept); err != nil {
			return nil, fmt.Errorf("scan employee dept: %w", err)
		}
		out[nik] = dept
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate employee dept: %w", err)
	}
	return out, nil
}
