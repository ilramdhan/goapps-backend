-- Reverse migration 000350: restore DELIVERY MARGIN → SALES.
BEGIN;

UPDATE bi_fact_metric
SET type = 'SALES'
WHERE type = 'DELIVERY MARGIN'
  AND source_id = (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE');

UPDATE bi_dashboard
SET filter_type = 'SALES'
WHERE dashboard_code = 'DELIVERY_MARGIN';

UPDATE bi_job
SET target_type = 'SALES',
    config = config
        || '{"target_type": "SALES", "source_view": "MGTDAT.VIEW_DELIVERY_MARGIN_MGT", "kind": "multi_metric"}'::jsonb
WHERE job_name = 'ETL_DELIVERY_MARGIN';

COMMIT;
