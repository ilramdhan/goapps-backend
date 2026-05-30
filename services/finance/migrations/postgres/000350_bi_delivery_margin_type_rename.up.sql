-- Migration 000350: Rename delivery margin fact data type SALES → DELIVERY MARGIN
-- to match the real Oracle MV FM_TYPE value (MGTDAT.MV_DASH_DELMAR_MGT returns
-- type='DELIVERY MARGIN'). The DELIVERY_MARGIN dashboard filter_type is updated
-- to match so chart queries continue to find their data.
BEGIN;

-- 1. Rename existing seeded delivery margin rows (loaded with type='SALES').
UPDATE bi_fact_metric
SET type = 'DELIVERY MARGIN'
WHERE type = 'SALES'
  AND source_id = (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE');

-- 2. Align dashboard filter_type to the real Oracle MV FM_TYPE value.
UPDATE bi_dashboard
SET filter_type = 'DELIVERY MARGIN'
WHERE dashboard_code = 'DELIVERY_MARGIN';

-- 3. Align the ETL job target_type + config to reflect the real MV type.
UPDATE bi_job
SET target_type = 'DELIVERY MARGIN',
    config = config
        || '{"target_type": "DELIVERY MARGIN", "source_view": "MGTDAT.MV_DASH_DELMAR_MGT", "kind": "etl_delivery_margin"}'::jsonb
WHERE job_name = 'ETL_DELIVERY_MARGIN';

COMMIT;
