-- P002 (DOWN): hapus job ETL_PP_PACK_DAILY (beserta lognya).
DELETE FROM bi_job_log WHERE job_id IN (SELECT job_id FROM bi_job WHERE job_name = 'ETL_PP_PACK_DAILY');
DELETE FROM bi_job WHERE job_name = 'ETL_PP_PACK_DAILY';
