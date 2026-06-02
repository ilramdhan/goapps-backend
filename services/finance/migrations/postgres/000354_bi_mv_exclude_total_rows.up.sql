-- Migration 000354: Rebuild bi_metric MVs to exclude Oracle pre-computed total rows.
--
-- Background:
--   MV_DASH_MIS_MGT now includes pre-computed total rows for each metric group:
--     group_1='EBITDA',     group_2='EBITDA',     group_3='EBITDA',     group_2_order=99
--     group_1='NET PROFIT', group_2='NET PROFIT',  group_3='NET PROFIT', group_2_order=99
--   Oracle uses this pattern specifically for summary rows — sub-categories never share
--   the same name as their parent category in any real P&L or margin report.
--
-- Problem:
--   Postgres MVs included these total rows alongside components, causing double-counting:
--     mv_bi_metric_g2 for EBITDA = SUM(all components) + total_row = 2 × actual EBITDA
--
-- Fix:
--   Exclude rows that are BOTH: (a) group_2 = group_1  AND  (b) group_2_order = 99.
--   Double-guard prevents false positives — a component would need BOTH the exact same
--   name as its parent AND order=99 to be excluded, which cannot happen in normal data.
--
-- Safety analysis:
--   ✅ Normal component rows: group_2 differs from group_1 (e.g. EBITDA → INCOME)
--   ✅ Even if a component shared a name, order < 99 keeps it safe
--   ✅ DELIVERY MARGIN: group_1=Local/Export ≠ group_2=FG/BLACK — unaffected
--   ✅ SALES: group_1=Local/Export ≠ group_2=BLACK/COLOR — unaffected
--   ✅ NET PROFIT: only rows with group_2='NET PROFIT' AND order=99 are excluded
--
-- After running: trigger MV_REFRESH from BI admin panel to pick up live ETL data.

BEGIN;

DROP MATERIALIZED VIEW IF EXISTS mv_bi_metric_g2 CASCADE;
DROP MATERIALIZED VIEW IF EXISTS mv_bi_metric_g1 CASCADE;

-- g1: aggregate by (type, group_1, metric_name, periode, scenario).
-- Guard: exclude rows where group_2 = group_1 AND group_2_order = 99.
-- These are Oracle pre-computed totals. Using group_2_order as a second condition
-- prevents accidentally excluding any data that merely shares a name.
CREATE MATERIALIZED VIEW mv_bi_metric_g1 AS
SELECT type, group_1, metric_name, metric_category,
       periode_grain, periode_date, scenario,
       SUM(display_value) AS value,
       MAX(group_1_order) AS group_1_order
FROM bi_fact_metric
WHERE is_active
  AND agg_method = 'SUM'
  AND NOT (group_2 IS NOT NULL AND group_2 = group_1 AND group_2_order = 99)
GROUP BY type, group_1, metric_name, metric_category, periode_grain, periode_date, scenario;

CREATE UNIQUE INDEX ux_mv_bi_g1
  ON mv_bi_metric_g1 (type, group_1, metric_name, periode_grain, periode_date, scenario);

-- g2: aggregate by (type, group_1, group_2, metric_name, periode, scenario).
-- Same double-guard: exclude only rows where group_2 = group_1 AND group_2_order = 99.
CREATE MATERIALIZED VIEW mv_bi_metric_g2 AS
SELECT type, group_1, group_2, metric_name, metric_category,
       periode_grain, periode_date, scenario,
       SUM(display_value) AS value,
       MAX(group_2_order) AS group_2_order
FROM bi_fact_metric
WHERE is_active
  AND agg_method = 'SUM'
  AND group_2 IS NOT NULL
  AND NOT (group_2 = group_1 AND group_2_order = 99)
GROUP BY type, group_1, group_2, metric_name, metric_category, periode_grain, periode_date, scenario;

CREATE UNIQUE INDEX ux_mv_bi_g2
  ON mv_bi_metric_g2 (type, group_1, group_2, metric_name, periode_grain, periode_date, scenario);

-- Refresh function (CONCURRENTLY requires the unique indexes above).
CREATE OR REPLACE FUNCTION bi_refresh_dashboard_mvs() RETURNS void AS $$
BEGIN
  REFRESH MATERIALIZED VIEW CONCURRENTLY mv_bi_metric_g1;
  REFRESH MATERIALIZED VIEW CONCURRENTLY mv_bi_metric_g2;
END;
$$ LANGUAGE plpgsql;

-- Initial refresh from current bi_fact_metric content.
REFRESH MATERIALIZED VIEW mv_bi_metric_g1;
REFRESH MATERIALIZED VIEW mv_bi_metric_g2;

COMMIT;
