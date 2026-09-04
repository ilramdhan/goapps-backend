-- Migration: Allow 'NONE' in chk_rm_cost_flag_valuation_used.
--
-- Context: SelectValuationWithFlag (calc_formulas_v2.go) previously resolved
-- the AUTO valuation cascade (CL→SL→FL→PR) to the misleading label "FL" when
-- every candidate was zero -- implying a fixed-landed-cost source was used
-- when in fact no price source existed for the period at all. The cascade
-- now resolves the all-zero case to the honest label "NONE" instead. The
-- chk_rm_cost_flag_valuation_used CHECK constraint (widened in migration
-- 000472) must be widened again to accept this new label, otherwise any V2
-- cascade resolving all-zero would violate the CHECK with SQLSTATE 23514.
--
-- chk_rm_cost_flag_marketing_used is untouched: the marketing cascade
-- (SelectMarketingWithFlag) is unaffected by this change.

ALTER TABLE cst_rm_cost
    DROP CONSTRAINT IF EXISTS chk_rm_cost_flag_valuation_used;

ALTER TABLE cst_rm_cost
    ADD CONSTRAINT chk_rm_cost_flag_valuation_used
    CHECK (flag_valuation_used IN (
        'CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT',
        'CR','SR','PR','CL','SL','FL','NONE'
    ));
