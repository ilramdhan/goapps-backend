-- Migration: create mv_bi_metric_g1/g2 materialized views + refresh function.
BEGIN;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_bi_metric_g1 AS
SELECT type, group_1, periode_grain, periode_date, scenario,
       SUM(display_value) AS value,
       MAX(group_1_order) AS group_1_order
FROM bi_fact_metric
WHERE is_active
GROUP BY type, group_1, periode_grain, periode_date, scenario;

CREATE UNIQUE INDEX IF NOT EXISTS ux_mv_bi_g1
    ON mv_bi_metric_g1 (type, group_1, periode_grain, periode_date, scenario);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_bi_metric_g2 AS
SELECT type, group_1, group_2, periode_grain, periode_date, scenario,
       SUM(display_value) AS value,
       MAX(group_2_order) AS group_2_order
FROM bi_fact_metric
WHERE is_active AND group_2 IS NOT NULL
GROUP BY type, group_1, group_2, periode_grain, periode_date, scenario;

CREATE UNIQUE INDEX IF NOT EXISTS ux_mv_bi_g2
    ON mv_bi_metric_g2 (type, group_1, group_2, periode_grain, periode_date, scenario);

CREATE OR REPLACE FUNCTION bi_refresh_dashboard_mvs() RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_bi_metric_g1;
    REFRESH MATERIALIZED VIEW CONCURRENTLY mv_bi_metric_g2;
END;
$$ LANGUAGE plpgsql;

COMMIT;
