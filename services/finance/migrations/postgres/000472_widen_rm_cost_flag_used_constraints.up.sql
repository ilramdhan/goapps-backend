-- Migration: Widen chk_rm_cost_flag_valuation_used / chk_rm_cost_flag_marketing_used
-- to accept the V2 cascade-resolved labels.
--
-- Context: ENG-RM-01/P3 (see calculate_handler_v2.go buildOrApplyCost) changed
-- flag_valuation_used / flag_marketing_used to store the V2 engine's resolved
-- cascade label (SelectValuationWithFlag / SelectMarketingWithFlag in
-- calc_formulas_v2.go) instead of the V1 stage enum. The DB constraints from
-- 000012_create_cst_rm_cost were never updated to match, so any V2 cascade
-- resolving to a V2-only label (CR/SR/PR/CL/SL/FL for valuation,
-- SP/PP/FP for marketing) violates the CHECK — this is what caused
-- RM_COST_CA-* jobs chained after oracle-sync to fail with SQLSTATE 23514.
--
-- flag_simulation_used is untouched: buildOrApplyCost always sets it from
-- head.FlagSimulation() (a V1 Stage), never from a V2 cascade selector.
--
-- Old V1 labels are kept in the allowed set for historical rows.

ALTER TABLE cst_rm_cost
    DROP CONSTRAINT IF EXISTS chk_rm_cost_flag_valuation_used;

ALTER TABLE cst_rm_cost
    ADD CONSTRAINT chk_rm_cost_flag_valuation_used
    CHECK (flag_valuation_used IN (
        'CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT',
        'CR','SR','PR','CL','SL','FL'
    ));

ALTER TABLE cst_rm_cost
    DROP CONSTRAINT IF EXISTS chk_rm_cost_flag_marketing_used;

ALTER TABLE cst_rm_cost
    ADD CONSTRAINT chk_rm_cost_flag_marketing_used
    CHECK (flag_marketing_used IN (
        'CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT',
        'SP','PP','FP'
    ));
