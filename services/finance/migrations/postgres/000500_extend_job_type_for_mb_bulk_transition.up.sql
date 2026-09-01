-- Migration: Extend chk_job_type to allow mb_bulk_transition jobs.
-- Context: Bulk MB Head Regenerate (Phase B) fans out one parent job.Execution
-- per bulk request (force_unvalidate/submit/validate, discriminated by the
-- existing subtype column) plus one child per mbh_id. Lowercase to match the
-- domain constant job.TypeMBBulkTransition.

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export', 'mb_bulk_transition'));
