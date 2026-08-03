-- 000466_add_chunk_progress_counted.up.sql
-- Idempotency key for cal_job progress accounting (P4d).
--
-- The orchestrator's advanceAfterChunk increments cal_job's
-- success/failed/blocked counters once per ChunkCompletedEvent. That event can
-- legitimately arrive more than once for the same chunk:
--   * the coordinator Nack-requeues on a transient DB error and RabbitMQ
--     redelivers the same message;
--   * the stuck-chunk sweeper publishes a synthetic completion, and the
--     worker it gave up on later publishes its own;
--   * the worker republishes on duplicate delivery of a terminal chunk
--     (processor.go handleChunk's idempotency branch).
-- Each duplicate used to re-increment, which is why production job 39 recorded
-- 48 processed chunks against 43 total and 4366 successes against 4166 products.
--
-- Chunk status cannot serve as the guard: both the worker (MarkCompleted) and
-- the sweeper (MarkChunkFailed) set the terminal status BEFORE publishing, so
-- by the time the coordinator sees the event the chunk is always terminal
-- already. A dedicated flag, flipped atomically in the same UPDATE that reads
-- it, is the only race-free key.
BEGIN;

ALTER TABLE cal_job_chunk
    ADD COLUMN IF NOT EXISTS cjc_progress_counted BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN cal_job_chunk.cjc_progress_counted IS
    'TRUE once this chunk''s counts have been folded into cal_job''s success/failed/blocked totals. Set atomically by the orchestrator so duplicate ChunkCompletedEvents cannot double-count.';

-- Existing rows: treat every already-terminal chunk as counted, so a redelivery
-- after this migration does not add its counts a second time on top of totals
-- that already include them. Non-terminal chunks stay FALSE and will be counted
-- normally when they complete.
UPDATE cal_job_chunk
   SET cjc_progress_counted = TRUE
 WHERE cjc_status IN ('SUCCESS', 'PARTIAL_FAILED', 'FAILED');

COMMIT;
