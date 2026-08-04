-- 000467_allow_skipped_chunk_status.down.sql
-- Restore the original constraint. Any SKIPPED rows are first folded back to
-- FAILED, otherwise the narrower CHECK cannot be re-applied.
BEGIN;

UPDATE cal_job_chunk
   SET cjc_status = 'FAILED'
 WHERE cjc_status = 'SKIPPED';

ALTER TABLE cal_job_chunk DROP CONSTRAINT IF EXISTS chk_cjc_status;

ALTER TABLE cal_job_chunk ADD CONSTRAINT chk_cjc_status
    CHECK (cjc_status IN ('QUEUED', 'DISPATCHED', 'PROCESSING', 'SUCCESS', 'PARTIAL_FAILED', 'FAILED'));

COMMIT;
