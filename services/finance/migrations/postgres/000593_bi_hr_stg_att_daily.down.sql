-- H016 (DOWN): hapus job ETL_HR_ATT_DAILY (+ log), fakta HR_HEADCOUNT, dan staging absensi.
BEGIN;
DELETE FROM bi_job_log WHERE job_id IN (SELECT job_id FROM bi_job WHERE job_name = 'ETL_HR_ATT_DAILY');
DELETE FROM bi_job WHERE job_name = 'ETL_HR_ATT_DAILY';
DELETE FROM bi_fact_metric WHERE type = 'HR_HEADCOUNT';
DROP TABLE IF EXISTS bi_hr_stg_att_daily;
COMMIT;
