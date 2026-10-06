-- 000555 down — restore the 000509 chk_job_type list.
--
-- If job_execution rows of type erp_integration / erp_master_sync exist, the
-- narrow CHECK fails and this down migration aborts with nothing changed;
-- job history is never deleted by a migration.

BEGIN;

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export',
                        'mb_bulk_transition', 'product_param_bulk'));

COMMIT;
