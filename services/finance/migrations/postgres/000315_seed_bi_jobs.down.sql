-- Remove seeded BI ETL jobs and their run history.
BEGIN;

DELETE FROM bi_job_log
WHERE job_id IN (SELECT job_id FROM bi_job WHERE job_name IN ('ETL_MIS_EBITDA', 'ETL_MIS_NET_PROFIT', 'MV_REFRESH'));

DELETE FROM bi_job WHERE job_name IN ('ETL_MIS_EBITDA', 'ETL_MIS_NET_PROFIT', 'MV_REFRESH');

COMMIT;
