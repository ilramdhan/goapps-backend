-- 000575 — extend chk_job_type with product_lookup_sync (master-lookup-sync P6b).
--
-- Keeps the 000555 list and adds:
--   product_lookup_sync — parent + one child per product re-applying master
--                         lookup values (cpp_filled_by = 'lookup_sync')
-- Pure widening; lock_timeout keeps the ALTER from queueing behind long
-- job_execution transactions.

BEGIN;

SET LOCAL lock_timeout = '5s';

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export',
                        'mb_bulk_transition', 'product_param_bulk', 'erp_integration', 'erp_master_sync',
                        'product_lookup_sync'));

COMMIT;
