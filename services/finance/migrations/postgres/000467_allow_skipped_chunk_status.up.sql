-- 000467_allow_skipped_chunk_status.up.sql
-- Widen chk_cjc_status to accept 'SKIPPED' (P4e).
--
-- CancelJobHandler calls MarkQueuedAsSkipped, which sets cjc_status = 'SKIPPED'
-- on chunks that were never dispatched, so a cancelled job leaves no orphaned
-- QUEUED rows. The original constraint from 000230 only allowed
-- QUEUED/DISPATCHED/PROCESSING/SUCCESS/PARTIAL_FAILED/FAILED, so that UPDATE
-- would abort with a 23514 check_violation.
--
-- 'SKIPPED' is deliberately distinct from 'FAILED': the chunk did not fail, it
-- was never attempted. cal_job_product already carries a 'SKIPPED' status for
-- the same reason (000231).
BEGIN;

ALTER TABLE cal_job_chunk DROP CONSTRAINT IF EXISTS chk_cjc_status;

ALTER TABLE cal_job_chunk ADD CONSTRAINT chk_cjc_status
    CHECK (cjc_status IN ('QUEUED', 'DISPATCHED', 'PROCESSING', 'SUCCESS', 'PARTIAL_FAILED', 'FAILED', 'SKIPPED'));

COMMIT;
