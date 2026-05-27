-- Migration: Extend chk_job_type to allow rm_cost_export jobs.
-- Context: 000013 added the whitelist of job types {oracle_sync, rm_cost_calculation}.
-- We now need an `rm_cost_export` job that the worker handles to render Excel
-- and upload to MinIO. Lowercase to match domain constants.

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export'));
