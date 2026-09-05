-- Migration: Allow 'NONE' in chk_rm_cost_flag_marketing_used.
--
-- Context: SelectMarketingWithFlag (calc_formulas_v2.go) previously resolved
-- the AUTO marketing cascade (SP→PP→FP) to the misleading label "FP" when
-- every candidate was zero -- implying a forecast-price source was used
-- when in fact no price source existed for the period at all. The cascade
-- now resolves the all-zero case to the honest label "NONE" instead. The
-- chk_rm_cost_flag_marketing_used CHECK constraint (widened in migration
-- 000472) must be widened again to accept this new label, otherwise any V2
-- cascade resolving all-zero would violate the CHECK with SQLSTATE 23514.
--
-- chk_rm_cost_flag_valuation_used is untouched: it was already widened for
-- 'NONE' in migration 000501 for the valuation cascade, which is unaffected
-- by this change.

ALTER TABLE cst_rm_cost
    DROP CONSTRAINT IF EXISTS chk_rm_cost_flag_marketing_used;

ALTER TABLE cst_rm_cost
    ADD CONSTRAINT chk_rm_cost_flag_marketing_used
    CHECK (flag_marketing_used IN (
        'CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT',
        'SP','PP','FP','NONE'
    ));
