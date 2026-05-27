-- Revert chk_job_type to the prior whitelist (without rm_cost_export).

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation'));
