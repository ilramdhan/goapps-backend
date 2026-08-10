-- Migration: Extend chk_job_type to allow product_cost_sheet_export jobs.
-- Context: 000025 added rm_cost_export to the whitelist {oracle_sync,
-- rm_cost_calculation, rm_cost_export}. We now need a
-- `product_cost_sheet_export` job that the worker handles to render the
-- 95-row product cost sheet Excel and upload it to MinIO. Lowercase to
-- match domain constants.

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export'));
