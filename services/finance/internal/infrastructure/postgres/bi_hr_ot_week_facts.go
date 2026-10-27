package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/hrot"
)

// r2WeeklyTypes are the WEEKLY fact types of R2; hrot.TypeWeekDaily is the DAILY one.
var r2WeeklyTypes = []string{hrot.TypeWeek, hrot.TypeWeekStatus, hrot.TypeWeekBucket, hrot.TypeWeekExcess, hrot.TypeChronic}

// nearLimitConfigKey is the bi_job.config key overriding the R2 early-warning threshold (hours).
const nearLimitConfigKey = "near_limit_hours"

// hrAggSum is the agg_method of additive HR facts.
const hrAggSum = "SUM"

// replaceOTWeekFacts recomputes the R2 facts (H012) of every ISO week touched by [d1, d2] from
// staging, inside the caller's transaction (HD-04): facts of those weeks are deleted and re-inserted.
// Over-limit streaks continue from the HR_OT_CHRONIC fact of the week before the window.
func replaceOTWeekFacts(ctx context.Context, tx *sql.Tx, d1, d2 time.Time, sourceID uuid.UUID,
	dept map[string]string, loadedFrom time.Time,
) error {
	firstWeek, lastWeek := hrot.Monday(d1), hrot.Monday(d2)
	rows, err := loadOTWeekStaging(ctx, tx, firstWeek, d2)
	if err != nil {
		return err
	}
	near, err := loadNearLimit(ctx, tx, sourceID)
	if err != nil {
		return err
	}
	prev, err := loadPrevStreak(ctx, tx, firstWeek.AddDate(0, 0, -7))
	if err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM bi_fact_metric
WHERE scenario = 'ACTUAL'
  AND ((type = ANY($1) AND periode_grain = $2 AND periode_date BETWEEN $3 AND $4)
    OR (type = $5 AND periode_grain = $6 AND periode_date BETWEEN $3 AND $7))`,
		pq.Array(r2WeeklyTypes), hrot.GrainWeekly, firstWeek, lastWeek, hrot.TypeWeekDaily, hrot.GrainDaily, d2); err != nil {
		return fmt.Errorf("delete OT week facts: %w", err)
	}

	facts := hrot.AggregateWeekly(rows, dept, hrot.WeeklyParams{
		From: firstWeek, To: d2, LoadedFrom: loadedFrom, NearLimit: near, PrevStreak: prev,
	})
	return insertHRFacts(ctx, tx, facts, sourceID)
}

// insertHRFacts inserts aggregated HR facts. Unit and aggregation follow the fact type.
func insertHRFacts(ctx context.Context, tx *sql.Tx, facts []hrot.Fact, sourceID uuid.UUID) error {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3, periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, metric_name, metric_category, agg_method,
    source_id, dimension_key, is_active
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::numeric,$8::numeric,$9,'ACTUAL','VALUE',$10,$11,$12,'',TRUE)`)
	if err != nil {
		return fmt.Errorf("prepare HR fact insert: %w", err)
	}
	defer func() {
		if cErr := stmt.Close(); cErr != nil {
			_ = cErr
		}
	}()

	for _, f := range facts {
		uom, category, agg := hrFactUnit(f.Type)
		if _, err = stmt.ExecContext(ctx, f.Type, f.Group1, biNullableString(f.Group2), biNullableString(f.Group3),
			f.Grain, f.PeriodDate, f.PeriodLabel, f.Value.String(), uom, category, agg, sourceID); err != nil {
			return fmt.Errorf("insert HR fact %s: %w", f.Type, err)
		}
	}
	return nil
}

// hrFactUnit returns uom, metric_category and agg_method of an HR fact type. The chronic streak
// is a running count, so it is not summable across weeks (LAST).
func hrFactUnit(factType string) (uom, category, agg string) {
	switch factType {
	case hrot.TypeWeekStatus, hrot.TypeWeekBucket, hrot.TypeEmpCount, hrot.TypeDQEmp, hrot.TypeOverWeeks, hrot.TypeOverEmp, hrot.TypeOverNik:
		return "PERSON", "COUNT", hrAggSum
	case hrot.TypeChronic:
		return "WEEKS", "COUNT", "LAST"
	default:
		return "HOURS", "VOLUME", hrAggSum
	}
}

func loadOTWeekStaging(ctx context.Context, tx *sql.Tx, from, to time.Time) ([]hrot.WeekRow, error) {
	rs, err := tx.QueryContext(ctx, `SELECT work_date, nik, COALESCE(dept_name, ''), ot_type, ot_calc_hours::text
FROM bi_hr_stg_ot_daily WHERE work_date BETWEEN $1 AND $2`, from, to)
	if err != nil {
		return nil, fmt.Errorf("read OT week staging: %w", err)
	}
	defer func() {
		if cErr := rs.Close(); cErr != nil {
			_ = cErr
		}
	}()

	var out []hrot.WeekRow
	for rs.Next() {
		var r hrot.WeekRow
		var calc string
		if err = rs.Scan(&r.WorkDate, &r.NIK, &r.StagingDept, &r.OTType, &calc); err != nil {
			return nil, fmt.Errorf("scan OT week staging: %w", err)
		}
		if r.CalcHours, err = decimal.NewFromString(calc); err != nil {
			return nil, fmt.Errorf("parse calc hours: %w", err)
		}
		r.WorkDate = time.Date(r.WorkDate.Year(), r.WorkDate.Month(), r.WorkDate.Day(), 0, 0, 0, 0, time.UTC)
		out = append(out, r)
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate OT week staging: %w", err)
	}
	return out, nil
}

// loadNearLimit reads the early-warning threshold from the config of the OT staging job of this
// source (PRD R2 "ambang bisa diatur"); absent or invalid → hrot.DefaultNearLimit.
func loadNearLimit(ctx context.Context, tx *sql.Tx, sourceID uuid.UUID) (decimal.Decimal, error) {
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT config->>$1 FROM bi_job
WHERE source_id = $2 AND target_type = 'HR_OT_STAGING' AND config->>$1 IS NOT NULL
ORDER BY job_name LIMIT 1`, nearLimitConfigKey, sourceID).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return decimal.Zero, fmt.Errorf("read near-limit config: %w", err)
	}
	if !raw.Valid {
		return hrot.DefaultNearLimit, nil
	}
	v, pErr := decimal.NewFromString(raw.String)
	if pErr != nil || !v.IsPositive() {
		return hrot.DefaultNearLimit, nil //nolint:nilerr // invalid config falls back to the PRD default
	}
	return v, nil
}

// loadPrevStreak returns the over-limit streak (HR_OT_CHRONIC) per NIK of the week starting monday.
func loadPrevStreak(ctx context.Context, tx *sql.Tx, monday time.Time) (map[string]int, error) {
	rs, err := tx.QueryContext(ctx, `SELECT group_3, value::int FROM bi_fact_metric
WHERE type = $1 AND periode_grain = $2 AND periode_date = $3 AND scenario = 'ACTUAL' AND group_3 IS NOT NULL`,
		hrot.TypeChronic, hrot.GrainWeekly, monday)
	if err != nil {
		return nil, fmt.Errorf("read previous streak: %w", err)
	}
	defer func() {
		if cErr := rs.Close(); cErr != nil {
			_ = cErr
		}
	}()

	out := map[string]int{}
	for rs.Next() {
		var nik string
		var n int
		if err = rs.Scan(&nik, &n); err != nil {
			return nil, fmt.Errorf("scan previous streak: %w", err)
		}
		out[nik] = n
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate previous streak: %w", err)
	}
	return out, nil
}

// rebuildOTOverMonthFacts rebuilds HR_OT_OVER_WEEKS and HR_OT_OVER_EMP (H038, HD-20) from the stored
// WEEKLY HR_OT_WEEK_STATUS facts: per dept and payroll month, the over-limit employee-weeks and the
// distinct employees. A week belongs to the payroll period of its Sunday (K10), the same rule as the
// viewer filter. All months are rebuilt so the dept restate (HD-15) of older weeks carries over; it runs
// after restateOTDept. Weeks never loaded (before staging starts) have no facts, so are not counted.
func rebuildOTOverMonthFacts(ctx context.Context, tx *sql.Tx, sourceID uuid.UUID) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bi_fact_metric
WHERE type IN ($1, $2, $4) AND periode_grain = $3 AND scenario = 'ACTUAL'`,
		hrot.TypeOverWeeks, hrot.TypeOverEmp, hrot.GrainMonthly, hrot.TypeOverNik); err != nil {
		return fmt.Errorf("delete OT over-limit month facts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3, periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, metric_name, metric_category, agg_method,
    source_id, dimension_key, is_active)
SELECT k.typ, m.group_1, NULL, NULL, $3, m.pe, to_char(m.pe, 'YYYYMM'),
       CASE k.typ WHEN $1 THEN m.weeks ELSE m.emps END,
       CASE k.typ WHEN $1 THEN m.weeks ELSE m.emps END,
       'PERSON', 'ACTUAL', 'VALUE', 'COUNT', 'SUM', $4, '', TRUE
FROM (
    SELECT group_1, pe, COUNT(*)::numeric AS weeks, COUNT(DISTINCT group_3)::numeric AS emps
    FROM (
        SELECT group_1, group_3,
               (date_trunc('month', periode_date + 6) + interval '24 days'
                 + CASE WHEN EXTRACT(day FROM periode_date + 6) >= 26 THEN interval '1 month' ELSE interval '0' END)::date AS pe
        FROM bi_fact_metric
        WHERE type = $5 AND periode_grain = $6 AND scenario = 'ACTUAL' AND group_2 = $7 AND group_3 IS NOT NULL
    ) w GROUP BY group_1, pe
) m CROSS JOIN (VALUES ($1), ($2)) AS k(typ)`,
		hrot.TypeOverWeeks, hrot.TypeOverEmp, hrot.GrainMonthly, sourceID,
		hrot.TypeWeekStatus, hrot.GrainWeekly, hrot.StatusOver); err != nil {
		return fmt.Errorf("insert OT over-limit month facts: %w", err)
	}
	// H042 / HD-22: per NIK, same period rule; SUM per dept·month equals HR_OT_OVER_WEEKS.
	if _, err := tx.ExecContext(ctx, `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3, periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario, metric_name, metric_category, agg_method,
    source_id, dimension_key, is_active)
SELECT $1, group_1, NULL, group_3, $2, pe, to_char(pe, 'YYYYMM'),
       COUNT(*)::numeric, COUNT(*)::numeric, 'PERSON', 'ACTUAL', 'VALUE', 'COUNT', 'SUM', $3, '', TRUE
FROM (
    SELECT group_1, group_3,
           (date_trunc('month', periode_date + 6) + interval '24 days'
             + CASE WHEN EXTRACT(day FROM periode_date + 6) >= 26 THEN interval '1 month' ELSE interval '0' END)::date AS pe
    FROM bi_fact_metric
    WHERE type = $4 AND periode_grain = $5 AND scenario = 'ACTUAL' AND group_2 = $6 AND group_3 IS NOT NULL
) w GROUP BY group_1, group_3, pe`,
		hrot.TypeOverNik, hrot.GrainMonthly, sourceID,
		hrot.TypeWeekStatus, hrot.GrainWeekly, hrot.StatusOver); err != nil {
		return fmt.Errorf("insert OT over-limit NIK month facts: %w", err)
	}
	return nil
}
