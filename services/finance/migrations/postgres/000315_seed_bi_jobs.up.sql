-- Seed: BI ETL job registry + 24h sample run history so the Admin > ETL Jobs tab is
-- populated. Real Oracle execution is wired when the data engineer's procedures are ready;
-- for now the scheduler/manual trigger records a job_log entry (see TriggerHandler).
BEGIN;

WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'ERP_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, is_active)
SELECT v.job_name, src.source_id, v.target_type, v.schedule_cron, v.oracle_procedure, TRUE
FROM src, (VALUES
  ('ETL_MIS_EBITDA',     'MIS', '0 0,4,8,12,16,20 * * *', 'SP_DASHBOARD_MIS_EBITDA_REFRESH'),
  ('ETL_MIS_NET_PROFIT', 'MIS', '0 0,4,8,12,16,20 * * *', 'SP_DASHBOARD_MIS_NET_PROFIT_REFRESH'),
  ('MV_REFRESH',         'ALL', '5 0,4,8,12,16,20 * * *', NULL)
) AS v(job_name, target_type, schedule_cron, oracle_procedure)
ON CONFLICT (job_name) DO NOTHING;

-- Sample run history (last 24h) for the admin panel, including one FAILED run with an error.
INSERT INTO bi_job_log (job_id, started_at, ended_at, status, rows_affected, error_message, triggered_by, duration_ms)
SELECT j.job_id, l.started_at, l.ended_at, l.status, l.rows_affected, l.error_message, l.triggered_by, l.duration_ms
FROM (VALUES
  ('ETL_MIS_EBITDA',     NOW() - INTERVAL '20 hours', NOW() - INTERVAL '20 hours' + INTERVAL '7 minutes',  'SUCCESS', 2128, NULL::text, 'SCHEDULER', 420000),
  ('ETL_MIS_NET_PROFIT', NOW() - INTERVAL '20 hours', NOW() - INTERVAL '20 hours' + INTERVAL '4 minutes',  'SUCCESS', 276,  NULL,       'SCHEDULER', 240000),
  ('MV_REFRESH',         NOW() - INTERVAL '20 hours', NOW() - INTERVAL '20 hours' + INTERVAL '40 seconds', 'SUCCESS', NULL, NULL,       'SCHEDULER', 40000),
  ('ETL_MIS_EBITDA',     NOW() - INTERVAL '12 hours', NOW() - INTERVAL '12 hours' + INTERVAL '7 minutes',  'SUCCESS', 2135, NULL,       'SCHEDULER', 420000),
  ('ETL_MIS_EBITDA',     NOW() - INTERVAL '8 hours',  NOW() - INTERVAL '8 hours'  + INTERVAL '12 seconds', 'FAILED',  0,    'ORA-12170: TNS connect timeout (retry succeeded next run)', 'SCHEDULER', 12000),
  ('ETL_MIS_EBITDA',     NOW() - INTERVAL '4 hours',  NOW() - INTERVAL '4 hours'  + INTERVAL '7 minutes',  'SUCCESS', 2135, NULL,       'SCHEDULER', 420000),
  ('ETL_MIS_NET_PROFIT', NOW() - INTERVAL '4 hours',  NOW() - INTERVAL '4 hours'  + INTERVAL '4 minutes',  'SUCCESS', 276,  NULL,       'SCHEDULER', 240000)
) AS l(job_name, started_at, ended_at, status, rows_affected, error_message, triggered_by, duration_ms)
JOIN bi_job j ON j.job_name = l.job_name;

COMMIT;
