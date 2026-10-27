package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// BiHRStagingRepository writes HRIS staging tables (bi_hr_stg_*, bi_hr_employee).
type BiHRStagingRepository struct {
	db *DB
}

// NewBiHRStagingRepository constructs the repository.
func NewBiHRStagingRepository(db *DB) *BiHRStagingRepository {
	return &BiHRStagingRepository{db: db}
}

// ReplaceOTDaily deletes staging rows with work_date in [d1, d2] and inserts rows, upserts the
// employee label table (HD-07), then re-aggregates the R1 facts of the window (H011) — all in one
// transaction (HD-04). Rows with empty NIK are skipped. sourceID is stamped on the facts.
// Returns staging rows inserted.
func (r *BiHRStagingRepository) ReplaceOTDaily(
	ctx context.Context, d1, d2 time.Time, rows []oracleinfra.OTDailyRow, sourceID uuid.UUID,
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

	// Serialize concurrent replaces (05:00 run vs manual/midday run) so facts are not doubled.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "bi_hr_replace:"+sourceID.String()); err != nil {
		return 0, fmt.Errorf("advisory lock: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_hr_stg_ot_daily WHERE work_date BETWEEN $1 AND $2`, d1, d2); err != nil {
		return 0, fmt.Errorf("delete window: %w", err)
	}

	const ins = `INSERT INTO bi_hr_stg_ot_daily (
    work_date, week_start, att_period, week_att_period, emp_sys_id, nik, division, dept_id, dept_name,
    schedule_code, ot_type, ot_tier_1, ot_tier_2, ot_tier_3, ot_actual_hours, ot_calc_hours, ot_limit_hours,
    company_id, company
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULLIF($18,''),NULLIF($19,''))`
	const emp = `INSERT INTO bi_hr_employee (nik, emp_sys_id, emp_name, division, dept_id, dept_name, company_id, company, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NOW())
ON CONFLICT (nik) DO UPDATE SET emp_sys_id = EXCLUDED.emp_sys_id, emp_name = EXCLUDED.emp_name,
    division = EXCLUDED.division, dept_id = EXCLUDED.dept_id, dept_name = EXCLUDED.dept_name,
    company_id = EXCLUDED.company_id, company = EXCLUDED.company, updated_at = NOW()`

	inserted := 0
	for _, x := range rows {
		if x.NIK == "" { // cannot be keyed to an employee; skipped (count = len(rows) - inserted)
			continue
		}
		if _, err = tx.ExecContext(ctx, ins,
			x.WorkDate, x.WeekStart, x.AttPeriod, x.WeekAttPeriod, x.EmpSysID, x.NIK, x.Division, x.DeptID, x.DeptName,
			x.ScheduleCode, x.OTType, x.Tier1.String(), x.Tier2.String(), x.Tier3.String(),
			x.ActualHours.String(), x.CalcHours.String(), x.LimitHours.String(), x.CompID, x.CompName); err != nil {
			return 0, fmt.Errorf("insert staging OT: %w", err)
		}
		inserted++
	}
	for _, x := range latestPerNIK(rows) {
		if _, err = tx.ExecContext(ctx, emp, x.NIK, x.EmpSysID, x.EmpName, x.Division, x.DeptID, x.DeptName, x.CompID, x.CompName); err != nil {
			return 0, fmt.Errorf("upsert employee: %w", err)
		}
	}
	if err = replaceOTFacts(ctx, tx, d1, d2, sourceID); err != nil {
		return 0, fmt.Errorf("aggregate OT facts: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return inserted, nil
}

// latestPerNIK returns, per NIK, the row with the latest work_date (HD-18): the employee's current
// name, division, dept and company (HD-19). On a tie the row read last wins, like the stg_id DESC backfill (000555). Rows with empty NIK are skipped; the
// result is ordered by NIK so the upserts are deterministic.
func latestPerNIK(rows []oracleinfra.OTDailyRow) []oracleinfra.OTDailyRow {
	idx := make(map[string]int, len(rows))
	out := make([]oracleinfra.OTDailyRow, 0)
	for _, x := range rows {
		if x.NIK == "" {
			continue
		}
		i, ok := idx[x.NIK]
		if !ok {
			idx[x.NIK] = len(out)
			out = append(out, x)
			continue
		}
		if !x.WorkDate.Before(out[i].WorkDate) {
			out[i] = x
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NIK < out[j].NIK })
	return out
}
