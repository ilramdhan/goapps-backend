-- H011: job harian 05:00 tarik VIEW_BI_OT_DAILY (35 hari) ke staging + agregasi R1 (HD-04, PRD §8).
-- Dispatch: config.kind etl_* → Router → HRISLoader (target_type HR_OT_STAGING). Idempotent.
WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT 'ETL_HR_OT_DAILY', src.source_id, 'HR_OT_STAGING', '0 5 * * *', NULL,
       '{"kind": "etl_hr_ot_staging", "source_view": "MGTHRIS.VIEW_BI_OT_DAILY", "target_type": "HR_OT_STAGING", "source_code": "HRIS_ORACLE"}'::jsonb,
       TRUE
FROM src
ON CONFLICT (job_name) DO NOTHING;
