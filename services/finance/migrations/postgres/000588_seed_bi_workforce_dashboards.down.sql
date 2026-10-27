-- H013 (DOWN): hapus dashboard & group WORKFORCE, job siang, kembalikan jadwal 05:00 server-TZ.
BEGIN;
DELETE FROM bi_job_log WHERE job_id IN (SELECT job_id FROM bi_job WHERE job_name = 'ETL_HR_OT_MIDDAY');
DELETE FROM bi_job WHERE job_name = 'ETL_HR_OT_MIDDAY';
UPDATE bi_job
SET schedule_cron = '0 5 * * *', config = config - 'refresh_mvs'
WHERE job_name = 'ETL_HR_OT_DAILY' AND schedule_cron = 'CRON_TZ=Asia/Jakarta 0 5 * * *';
DELETE FROM bi_dashboard
WHERE dashboard_code IN ('HR_OT_DEPT', 'HR_OT_TREND', 'HR_OT_OVERLIMIT', 'HR_OT_WEEK_HIST', 'HR_OT_WEEK_DETAIL');
DELETE FROM bi_dashboard_group g
WHERE g.group_code = 'WORKFORCE' AND NOT EXISTS (SELECT 1 FROM bi_dashboard d WHERE d.group_id = g.group_id);
COMMIT;
