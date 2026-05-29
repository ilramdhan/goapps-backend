BEGIN;

WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT
  'ETL_DELIVERY_MARGIN',
  src.source_id,
  'SALES',
  '0 1,7,13,19 * * *',
  NULL,
  '{"oracle_source":"MGTDAT.VIEW_DELIVERY_MARGIN_MGT","target_type":"SALES","source_code":"ERP_ORACLE","sign_flip_in_source":true,"periode_filter":"previous_month","kind":"multi_metric"}'::jsonb,
  TRUE
FROM src
ON CONFLICT (job_name) DO NOTHING;

COMMIT;
