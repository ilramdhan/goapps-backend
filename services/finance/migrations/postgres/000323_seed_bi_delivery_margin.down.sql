BEGIN;
DELETE FROM bi_fact_metric
WHERE type = 'SALES'
  AND source_id = (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE');

-- Restore the interim constraint (only makes sense if MIS data is also cleared; kept for symmetry).
ALTER TABLE bi_fact_metric
  ADD CONSTRAINT IF NOT EXISTS uq_bi_fm_bk_itest UNIQUE NULLS NOT DISTINCT
  (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key);

REFRESH MATERIALIZED VIEW mv_bi_metric_g1;
REFRESH MATERIALIZED VIEW mv_bi_metric_g2;
COMMIT;
