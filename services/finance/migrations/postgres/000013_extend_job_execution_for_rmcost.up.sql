-- Migration: Add chk_job_type to job_execution and include new rm_cost_calculation type.
-- Context: 000008 created job_execution without a job_type whitelist; the Oracle sync
-- feature stores the lowercase token 'oracle_sync' (see job.TypeOracleSync in Go). We
-- now formalize the allowed set and add 'rm_cost_calculation' for the raw-material
-- landed-cost calculation job. Values are lowercase to match the domain constants.

-- DROP IF EXISTS keeps this migration idempotent in case chk_job_type was added ad-hoc.
ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

-- Normalize any legacy uppercase values written before the lowercase constants
-- were introduced (early Oracle-sync runs stored 'ORACLE_SYNC').
UPDATE job_execution SET job_type = LOWER(job_type) WHERE job_type <> LOWER(job_type);

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation'));

-- Composite index speeds up filtering the job history page by type + status.
CREATE INDEX IF NOT EXISTS idx_job_execution_type_status
    ON job_execution (job_type, status);
