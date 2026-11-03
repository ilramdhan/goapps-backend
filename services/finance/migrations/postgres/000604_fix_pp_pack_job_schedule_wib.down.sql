-- P011 (DOWN): kembalikan cron polos (UTC) dan hapus index.
BEGIN;
UPDATE bi_job SET schedule_cron = '0 7 * * *' WHERE job_name = 'ETL_PP_PACK_DAILY';
UPDATE bi_job SET schedule_cron = '15 7 * * *' WHERE job_name = 'MV_REFRESH_PP_PACK';
DROP INDEX IF EXISTS idx_bi_pp_stg_pack_prod_daily_merge;
DROP INDEX IF EXISTS idx_bi_pp_stg_pack_prod_daily_mc;
COMMIT;
