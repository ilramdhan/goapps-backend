-- Revert chk_rm_cost_flag_marketing_used to the 000472 whitelist (without
-- 'NONE').
--
-- WARNING: if any row was written with flag_marketing_used = 'NONE' while
-- this migration was applied, this rollback will fail with SQLSTATE 23514
-- until those rows are deleted or corrected.

ALTER TABLE cst_rm_cost
    DROP CONSTRAINT IF EXISTS chk_rm_cost_flag_marketing_used;

ALTER TABLE cst_rm_cost
    ADD CONSTRAINT chk_rm_cost_flag_marketing_used
    CHECK (flag_marketing_used IN (
        'CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT',
        'SP','PP','FP'
    ));
