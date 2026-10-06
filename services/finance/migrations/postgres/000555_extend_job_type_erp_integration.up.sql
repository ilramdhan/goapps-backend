-- 000555 — extend chk_job_type with the ERP integration job types
-- (design Part 1 §4.8, D-J1; plan-01 P0-T5).
--
-- Keeps the 000509 list and adds:
--   erp_integration  — batch steps (load demand, coverage, derive, validate,
--                      push, ADJ ops, recon, lock) and the attribute backfill
--                      subtype (D-J1)
--   erp_master_sync  — read-only OM_ITEM / OM_GRADE_CODE_1 replica sync
--                      (P0-T15) and its apply_grade_groups subtype (P0-T15b)
-- Lowercase, matching the existing job domain constants. Pure widening.

BEGIN;

ALTER TABLE job_execution
    DROP CONSTRAINT IF EXISTS chk_job_type;

ALTER TABLE job_execution
    ADD CONSTRAINT chk_job_type
    CHECK (job_type IN ('oracle_sync', 'rm_cost_calculation', 'rm_cost_export', 'product_cost_sheet_export',
                        'mb_bulk_transition', 'product_param_bulk', 'erp_integration', 'erp_master_sync'));

COMMIT;
