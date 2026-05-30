-- Migration 000351: Seed BI ETL job entries for the three Oracle MV loaders.
-- Each job maps to a config["kind"] value that TriggerHandler dispatches to
-- the corresponding MVLoader method (LoadMIS / LoadDeliveryMargin / LoadSales).
-- ON CONFLICT updates config + target_type so re-runs are idempotent.
BEGIN;

-- ETL job for MIS MV (EBITDA / Net Profit data).
WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT
    'ETL_MIS_MV',
    src.source_id,
    'MIS',
    '0 1 * * *',
    NULL,
    '{"kind": "etl_mis", "source_view": "MGTDAT.MV_DASH_MIS_MGT", "target_type": "MIS"}'::jsonb,
    TRUE
FROM src
ON CONFLICT (job_name) DO UPDATE SET
    target_type = EXCLUDED.target_type,
    config      = EXCLUDED.config,
    updated_at  = NOW();

-- ETL job for Delivery Margin MV (updates the existing ETL_DELIVERY_MARGIN job kind).
-- The existing ETL_DELIVERY_MARGIN job (seeded in 000331) had kind='multi_metric';
-- we update its config so TriggerHandler now dispatches it via the real Oracle ETL path.
UPDATE bi_job
SET config = '{"kind": "etl_delivery_margin", "source_view": "MGTDAT.MV_DASH_DELMAR_MGT", "target_type": "DELIVERY MARGIN"}'::jsonb,
    updated_at = NOW()
WHERE job_name = 'ETL_DELIVERY_MARGIN';

-- Insert if the DELIVERY_MARGIN job does not yet exist (fresh install).
WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT
    'ETL_DELIVERY_MARGIN',
    src.source_id,
    'DELIVERY MARGIN',
    '0 2 * * *',
    NULL,
    '{"kind": "etl_delivery_margin", "source_view": "MGTDAT.MV_DASH_DELMAR_MGT", "target_type": "DELIVERY MARGIN"}'::jsonb,
    TRUE
FROM src
WHERE NOT EXISTS (SELECT 1 FROM bi_job WHERE job_name = 'ETL_DELIVERY_MARGIN');

-- ETL job for Sales MV (DAILY + MONTHLY grains).
WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT
    'ETL_SALES_MV',
    src.source_id,
    'SALES',
    '0 3 * * *',
    NULL,
    '{"kind": "etl_sales", "source_view": "MGTDAT.MV_DASH_SALES_MGT", "target_type": "SALES"}'::jsonb,
    TRUE
FROM src
ON CONFLICT (job_name) DO UPDATE SET
    target_type = EXCLUDED.target_type,
    config      = EXCLUDED.config,
    updated_at  = NOW();

COMMIT;
