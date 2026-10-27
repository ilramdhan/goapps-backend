-- H011 (DOWN): hapus job ETL_HR_OT_DAILY beserta log run-nya.
DELETE FROM bi_job_log WHERE job_id IN (SELECT job_id FROM bi_job WHERE job_name = 'ETL_HR_OT_DAILY');
DELETE FROM bi_job WHERE job_name = 'ETL_HR_OT_DAILY';
