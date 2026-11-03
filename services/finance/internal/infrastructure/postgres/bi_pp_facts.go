package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ppDeleteFactsSQL clears the PP_PACK_* facts of the window, widened to the start of d1's month for
// WEEKLY (back to the Monday of d1's week) and MONTHLY (ETL§D). $1 = d1, $2 = d2 as 'YYYY-MM-DD'.
const ppDeleteFactsSQL = `DELETE FROM bi_fact_metric
 WHERE type IN ('PP_PACK_SUMMARY', 'PP_PACK_PRODUCT', 'PP_PACK_MACHINE', 'PP_PACK_POLY')
   AND scenario = 'ACTUAL'
   AND (   (periode_grain = 'DAILY'   AND periode_date BETWEEN $1::date AND $2::date)
        OR (periode_grain = 'WEEKLY'  AND periode_date BETWEEN date_trunc('week', $1::date)::date AND $2::date)
        OR (periode_grain = 'MONTHLY' AND periode_date BETWEEN date_trunc('month', $1::date)::date AND $2::date))`

// ppInsertFactsSQL is ETL§D: one aggregate row per (type, group,
// dimension, grain, period) from the full staging data, unpivoted to the 5 PACK_* metrics.
// $1 = d1, $2 = d2 as 'YYYY-MM-DD', $3 = source_id.
const ppInsertFactsSQL = `INSERT INTO bi_fact_metric (
    type, group_1, group_2, group_3,
    group_1_order, group_2_order, group_3_order,
    periode_grain, periode_date, periode_label,
    value, display_value, uom, scenario,
    metric_name, metric_category, agg_method,
    source_id, dimension_key, loaded_at, is_active
)
SELECT a.type, a.group_1, a.group_2, a.group_3,
       a.group_1_order, a.group_2_order, a.group_3_order,
       a.periode_grain, a.periode_date, a.periode_label,
       m.value, m.value, 'KGS', 'ACTUAL',
       m.metric_name, 'VOLUME', 'SUM',
       $3::uuid, a.dimension_key, now(), TRUE
  FROM (
        SELECT t.type,
               s.dept_name                                                          AS group_1,
               CASE WHEN t.label_mc THEN s.transaction_type_mc ELSE s.transaction_type END AS group_2,
               s.group_type                                                         AS group_3,
               MIN (CASE s.dept_id WHEN 'TWT' THEN 1 WHEN 'TXT' THEN 2 ELSE 9 END)  AS group_1_order,
               MIN (s.prd_type_order)                                               AS group_2_order,
               MIN (s.group_type_order)                                             AS group_3_order,
               g.periode_grain,
               CASE g.periode_grain WHEN 'DAILY' THEN s.pack_date WHEN 'WEEKLY' THEN s.week_start ELSE s.month_start END AS periode_date,
               CASE g.periode_grain WHEN 'DAILY' THEN to_char (s.pack_date, 'YYYY-MM-DD')
                                    WHEN 'WEEKLY' THEN to_char (s.week_start, 'IYYY-"W"IW') ELSE s.period END AS periode_label,
               CASE t.type
                   WHEN 'PP_PACK_PRODUCT' THEN COALESCE (s.merge_no, '')
                   WHEN 'PP_PACK_MACHINE' THEN COALESCE (s.mc_no, '')
                   WHEN 'PP_PACK_POLY'    THEN COALESCE (s.poly_type, '')
                   ELSE ''
               END                                                                  AS dimension_key,
               SUM (s.total_qty) AS total_qty,
               SUM (s.ax_qty)    AS ax_qty,
               SUM (s.fq_qty)    AS fq_qty,
               SUM (s.dg_qty)    AS dg_qty,
               SUM (s.jlt_qty)   AS jlt_qty
          FROM bi_pp_stg_pack_prod_daily s
               CROSS JOIN (VALUES ('DAILY'), ('WEEKLY'), ('MONTHLY')) g (periode_grain)
               -- filter per type = old mail alert report filter
               JOIN (VALUES ('PP_PACK_SUMMARY', FALSE),
                            ('PP_PACK_PRODUCT', FALSE),
                            ('PP_PACK_MACHINE', TRUE),
                            ('PP_PACK_POLY',    TRUE)) t (type, label_mc)
                 ON    t.type IN ('PP_PACK_SUMMARY', 'PP_PACK_POLY')
                    OR (t.type = 'PP_PACK_PRODUCT' AND s.group_type NOT IN ('Boughtout', 'Rewinding'))
                    OR (t.type = 'PP_PACK_MACHINE' AND s.grade_eligible = 'Y')
         WHERE (   (g.periode_grain = 'DAILY'   AND s.pack_date   BETWEEN $1::date AND $2::date)
                OR (g.periode_grain = 'WEEKLY'  AND s.week_start  BETWEEN date_trunc('week', $1::date)::date AND $2::date)
                OR (g.periode_grain = 'MONTHLY' AND s.month_start BETWEEN date_trunc('month', $1::date)::date AND $2::date))
      GROUP BY 1, 2, 3, 4, 8, 9, 10, 11
       ) a
       CROSS JOIN LATERAL (VALUES ('PACK_TOTAL', a.total_qty),
                                  ('PACK_AX',    a.ax_qty),
                                  ('PACK_FQ',    a.fq_qty),
                                  ('PACK_DG',    a.dg_qty),
                                  ('PACK_JLT',   a.jlt_qty)) m (metric_name, value)`

// ppReconSQL is ETL§F (PP-08): PP_PACK_SUMMARY PACK_TOTAL per grain/period vs the staging
// sum. Every returned row is a mismatch. $1 = d1, $2 = d2 as 'YYYY-MM-DD'.
const ppReconSQL = `SELECT COALESCE (f.periode_grain, s.periode_grain), COALESCE (f.periode_date, s.periode_date)::text,
       COALESCE (f.fact_total, 0)::text, COALESCE (s.stg_total, 0)::text
  FROM (  SELECT periode_grain, periode_date, SUM (value) AS fact_total
            FROM bi_fact_metric
           WHERE type = 'PP_PACK_SUMMARY' AND metric_name = 'PACK_TOTAL' AND scenario = 'ACTUAL'
             -- per-grain window, same as the staging side (PP-08: the spec's single month-start
             -- bound reports DAILY facts before d1 as mismatches)
             AND (   (periode_grain = 'DAILY'   AND periode_date BETWEEN $1::date AND $2::date)
                  OR (periode_grain = 'WEEKLY'  AND periode_date BETWEEN date_trunc('week', $1::date)::date AND $2::date)
                  OR (periode_grain = 'MONTHLY' AND periode_date BETWEEN date_trunc('month', $1::date)::date AND $2::date))
        GROUP BY periode_grain, periode_date) f
       FULL JOIN
       (  SELECT g.periode_grain,
                 CASE g.periode_grain WHEN 'DAILY' THEN pack_date WHEN 'WEEKLY' THEN week_start ELSE month_start END AS periode_date,
                 SUM (total_qty) AS stg_total
            FROM bi_pp_stg_pack_prod_daily
                 CROSS JOIN (VALUES ('DAILY'), ('WEEKLY'), ('MONTHLY')) g (periode_grain)
           WHERE (   (g.periode_grain = 'DAILY'   AND pack_date   BETWEEN $1::date AND $2::date)
                  OR (g.periode_grain = 'WEEKLY'  AND week_start  BETWEEN date_trunc('week', $1::date)::date AND $2::date)
                  OR (g.periode_grain = 'MONTHLY' AND month_start BETWEEN date_trunc('month', $1::date)::date AND $2::date))
        GROUP BY 1, 2) s
       ON s.periode_grain = f.periode_grain AND s.periode_date = f.periode_date
 WHERE COALESCE (f.fact_total, 0) <> COALESCE (s.stg_total, 0)
 ORDER BY 1, 2`

// replacePPFacts recomputes the DAILY, WEEKLY and MONTHLY PP_PACK_* facts of [d1, d2] from staging inside
// the caller's transaction (ETL§D, PP-06): facts of the window are deleted and re-inserted, so
// combinations removed at the source disappear. WEEKLY/MONTHLY cover every week/month the window touches,
// summed from the full staging data. The ETL§F reconciliation then runs in the same transaction;
// any mismatch aborts it. Returns fact rows inserted.
func replacePPFacts(ctx context.Context, tx *sql.Tx, d1, d2 time.Time, sourceID uuid.UUID) (int64, error) {
	p1, p2 := d1.Format(time.DateOnly), d2.Format(time.DateOnly)
	if _, err := tx.ExecContext(ctx, ppDeleteFactsSQL, p1, p2); err != nil {
		return 0, fmt.Errorf("delete facts: %w", err)
	}
	res, err := tx.ExecContext(ctx, ppInsertFactsSQL, p1, p2, sourceID)
	if err != nil {
		return 0, fmt.Errorf("insert facts: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("facts rows affected: %w", err)
	}
	if err = reconcilePPFacts(ctx, tx, p1, p2); err != nil {
		return 0, err
	}
	return n, nil
}

// reconcilePPFacts runs ETL§F and returns an error listing the first mismatches, if any.
func reconcilePPFacts(ctx context.Context, tx *sql.Tx, p1, p2 string) error {
	rows, err := tx.QueryContext(ctx, ppReconSQL, p1, p2)
	if err != nil {
		return fmt.Errorf("reconcile facts: %w", err)
	}
	defer closeRows(rows)
	var diffs []string
	count := 0
	for rows.Next() {
		var grain, date, fact, stg string
		if err = rows.Scan(&grain, &date, &fact, &stg); err != nil {
			return fmt.Errorf("scan reconcile row: %w", err)
		}
		count++
		if len(diffs) < 5 {
			diffs = append(diffs, fmt.Sprintf("%s %s fact=%s staging=%s", grain, date, fact, stg))
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("reconcile rows: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("reconcile: %d period(s) differ between staging and facts: %s", count, strings.Join(diffs, "; "))
	}
	return nil
}
