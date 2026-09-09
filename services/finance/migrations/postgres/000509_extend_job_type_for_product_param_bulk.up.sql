-- Migration: Extend chk_job_type to allow product_param_bulk jobs.
-- Context: Bulk Edit Product Params (F4) fans out one parent job.Execution per
-- bulk request plus one child job.Execution per targeted
-- cost_product_master.cpm_product_sys_id, mirroring migration
-- 000500_extend_job_type_for_mb_bulk_transition.up.sql's pattern exactly for
-- Bulk MB Head Regenerate. Lowercase to match the domain constant
-- job.TypeProductParamBulk.

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export', 'mb_bulk_transition', 'product_param_bulk'));
