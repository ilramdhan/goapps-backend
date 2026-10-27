package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hratt"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hrot"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// ReplaceATTDaily deletes staging rows with work_date in [d1, d2], inserts rows, then recomputes the
// HR_HEADCOUNT facts of every payroll period touched by the window (H016) — all in one transaction
// (HD-04). Rows with empty NIK are skipped. Returns staging rows inserted.
func (r *BiHRStagingRepository) ReplaceATTDaily(
	ctx context.Context, d1, d2 time.Time, rows []oracleinfra.ATTDailyRow, sourceID uuid.UUID,
) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "bi_hr_replace_att:"+sourceID.String()); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_hr_stg_att_daily WHERE work_date BETWEEN $1 AND $2`, d1, d2); err != nil {
		return 0, fmt.Errorf("delete window: %w", err)
	}

	const ins = `INSERT INTO bi_hr_stg_att_daily (
    work_date, week_start, att_period, emp_sys_id, nik, division, dept_id, dept_name, emp_status, emp_group,
    schedule_code, sched_status, sched_type, sched_in, sched_out, actual_in, actual_out,
    actual_hour, day_cat_rekap, day_cat, is_scheduled_work, late_minutes, early_leave_minutes
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`

	inserted := 0
	for _, x := range rows {
		if x.NIK == "" {
			continue
		}
		if _, err = tx.ExecContext(ctx, ins,
			x.WorkDate, x.WeekStart, x.AttPeriod, x.EmpSysID, x.NIK, x.Division, x.DeptID, x.DeptName, x.EmpStatus, x.EmpGroup,
			x.ScheduleCode, x.SchedStatus, x.SchedType,
			attNullTime(x.SchedIn), attNullTime(x.SchedOut), attNullTime(x.ActualIn), attNullTime(x.ActualOut),
			x.ActualHour.String(), x.DayCatRekap, x.DayCat, x.IsScheduledWork,
			x.LateMinutes.String(), x.EarlyLeaveMinutes.String()); err != nil {
			return 0, fmt.Errorf("insert staging ATT: %w", err)
		}
		inserted++
	}
	if err = replaceHeadcountFacts(ctx, tx, d1, d2, sourceID); err != nil {
		return 0, fmt.Errorf("aggregate headcount facts: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}

func attNullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// replaceHeadcountFacts recomputes HR_HEADCOUNT of the payroll periods touched by [d1, d2]: DAILY
// facts inside the window, MONTHLY facts for periods staging holds completely (same rule as R1).
func replaceHeadcountFacts(ctx context.Context, tx *sql.Tx, d1, d2 time.Time, sourceID uuid.UUID) error {
	from, to := hrot.PayrollPeriodStart(d1), hrot.PayrollPeriodEnd(d2)

	rows, err := loadATTStaging(ctx, tx, from, to)
	if err != nil {
		return err
	}
	dept, err := loadCurrentDept(ctx, tx)
	if err != nil {
		return err
	}
	var loadedFrom time.Time
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(work_date), $1) FROM bi_hr_stg_att_daily`, from).
		Scan(&loadedFrom); err != nil {
		return fmt.Errorf("read staging start: %w", err)
	}
	loadedFrom = time.Date(loadedFrom.Year(), loadedFrom.Month(), loadedFrom.Day(), 0, 0, 0, 0, time.UTC)

	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_fact_metric
WHERE type = $1 AND scenario = 'ACTUAL'
  AND ((periode_grain = 'DAILY' AND periode_date BETWEEN $2 AND $3)
    OR (periode_grain = 'MONTHLY' AND periode_date BETWEEN $4 AND $3))`,
		hratt.TypeHeadcount, from, to, hrot.PayrollPeriodEnd(d1)); err != nil {
		return fmt.Errorf("delete headcount facts: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3, periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, metric_name, metric_category, agg_method,
    source_id, dimension_key, is_active
) VALUES ($1,$2,$3,NULL,$4,$5,$6,$7::numeric,$7::numeric,'PERSON','ACTUAL','VALUE','VOLUME','SUM',$8,'',TRUE)`)
	if err != nil {
		return fmt.Errorf("prepare headcount insert: %w", err)
	}
	defer func() {
		if cErr := stmt.Close(); cErr != nil {
			_ = cErr
		}
	}()
	for _, f := range hratt.Aggregate(rows, dept, from, to, loadedFrom) {
		if _, err = stmt.ExecContext(ctx, f.Type, f.Group1, biNullableString(f.Group2),
			f.Grain, f.PeriodDate, f.PeriodLabel, f.Value.String(), sourceID); err != nil {
			return fmt.Errorf("insert headcount fact: %w", err)
		}
	}
	return nil
}

func loadATTStaging(ctx context.Context, tx *sql.Tx, from, to time.Time) ([]hratt.Row, error) {
	rs, err := tx.QueryContext(ctx, `SELECT work_date, nik, COALESCE(dept_name, ''), COALESCE(emp_status, '')
FROM bi_hr_stg_att_daily WHERE work_date BETWEEN $1 AND $2`, from, to)
	if err != nil {
		return nil, fmt.Errorf("read ATT staging: %w", err)
	}
	defer func() {
		if cErr := rs.Close(); cErr != nil {
			_ = cErr
		}
	}()
	var out []hratt.Row
	for rs.Next() {
		var r hratt.Row
		if err = rs.Scan(&r.WorkDate, &r.NIK, &r.Dept, &r.Status); err != nil {
			return nil, fmt.Errorf("scan ATT staging: %w", err)
		}
		r.WorkDate = time.Date(r.WorkDate.Year(), r.WorkDate.Month(), r.WorkDate.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, r)
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate ATT staging: %w", err)
	}
	return out, nil
}
