-- P011 / PP-03: jadwal job packed production dalam WIB (000602/000603 memakai cron polos = UTC,
-- sehingga jalan 14:00/14:15 WIB). Plus index staging untuk drill produk/mesin. Idempotent.
BEGIN;

UPDATE bi_job SET schedule_cron = 'CRON_TZ=Asia/Jakarta 0 7 * * *'
 WHERE job_name = 'ETL_PP_PACK_DAILY' AND schedule_cron IN ('0 7 * * *', 'CRON_TZ=Asia/Jakarta 0 7 * * *');
UPDATE bi_job SET schedule_cron = 'CRON_TZ=Asia/Jakarta 15 7 * * *'
 WHERE job_name = 'MV_REFRESH_PP_PACK' AND schedule_cron IN ('15 7 * * *', 'CRON_TZ=Asia/Jakarta 15 7 * * *');

DO $$
BEGIN
    IF (SELECT count(*) FROM bi_job WHERE job_name IN ('ETL_PP_PACK_DAILY', 'MV_REFRESH_PP_PACK')
                                      AND schedule_cron LIKE 'CRON_TZ=Asia/Jakarta %') <> 2 THEN
        RAISE EXCEPTION 'P011: job ETL_PP_PACK_DAILY / MV_REFRESH_PP_PACK tidak ditemukan (jalankan 000602/000603 dulu)';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_bi_pp_stg_pack_prod_daily_merge ON bi_pp_stg_pack_prod_daily (merge_no, pack_date);
CREATE INDEX IF NOT EXISTS idx_bi_pp_stg_pack_prod_daily_mc    ON bi_pp_stg_pack_prod_daily (mc_no, pack_date);

COMMIT;
