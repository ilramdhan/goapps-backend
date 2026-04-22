-- Rollback: drop chk_job_type and the type/status index.

DROP INDEX IF EXISTS idx_job_execution_type_status;

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;
