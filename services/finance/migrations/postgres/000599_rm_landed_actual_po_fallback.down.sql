-- 000599_rm_landed_actual_po_fallback.down.sql
--
-- Restores F_YARN_RM_LANDED's expression to the exact 000519 value (ACTUAL
-- cascade CL -> SL -> FL without the PP fallback). Note: the Go default
-- DefaultRMLandedOrderActual still includes PP after a rollback of only this
-- migration; an explicit ACTUAL section here overrides it, so this row alone
-- fully restores the pre-000599 ACTUAL behavior.
UPDATE mst_formula
SET expression = 'ACTUAL:CL,SL,FL;FORECAST:SP,PP,FP;SELLING:SP,PP,FP',
    updated_at = NOW(),
    updated_by = 'migration_000599_down'
WHERE formula_code = 'F_YARN_RM_LANDED';
