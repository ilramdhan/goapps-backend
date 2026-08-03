-- 000466_add_chunk_progress_counted.down.sql
BEGIN;

ALTER TABLE cal_job_chunk DROP COLUMN IF EXISTS cjc_progress_counted;

COMMIT;
