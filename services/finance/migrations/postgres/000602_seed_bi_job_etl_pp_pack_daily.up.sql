-- P002 / PP-03: job ETL Packed Production, source HRIS_ORACLE, 07:00 WIB. Idempotent.
BEGIN;

WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT
    'ETL_PP_PACK_DAILY',
    src.source_id,
    'PP_PACK',
    '0 7 * * *',
    NULL,
    '{"kind": "etl_pp_pack_daily", "source_view": "ASPAK.VIEW_BI_PACK_PROD_DAILY", "target_type": "PP_PACK", "window_days": 35}'::jsonb,
    TRUE
FROM src
ON CONFLICT (job_name) DO NOTHING;

COMMIT;
