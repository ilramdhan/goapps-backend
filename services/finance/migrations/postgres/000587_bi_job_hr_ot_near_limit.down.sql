-- H012: hapus ambang near_limit_hours dari config job ETL_HR_OT_DAILY (agregasi kembali ke default 10).
UPDATE bi_job SET config = config - 'near_limit_hours' WHERE job_name = 'ETL_HR_OT_DAILY';
