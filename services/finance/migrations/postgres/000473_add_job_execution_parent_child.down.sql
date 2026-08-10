-- Rollback: restore the original idx_job_execution_active_unique scope and
-- drop the parent/child batch tracking columns.

DROP INDEX IF EXISTS idx_job_execution_active_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_job_execution_active_unique
    ON job_execution(job_type, period)
    WHERE status IN ('QUEUED', 'PROCESSING');

DROP INDEX IF EXISTS idx_job_execution_parent_job_id;

ALTER TABLE job_execution
    DROP COLUMN IF EXISTS jex_failed_children,
    DROP COLUMN IF EXISTS jex_completed_children,
    DROP COLUMN IF EXISTS jex_total_children,
    DROP COLUMN IF EXISTS jex_parent_job_id;
