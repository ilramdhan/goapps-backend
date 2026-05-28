-- Remove the ERP reference seed fact data.
BEGIN;
DELETE FROM bi_fact_metric WHERE source_id = (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE');
REFRESH MATERIALIZED VIEW mv_bi_metric_g1;
REFRESH MATERIALIZED VIEW mv_bi_metric_g2;
COMMIT;
