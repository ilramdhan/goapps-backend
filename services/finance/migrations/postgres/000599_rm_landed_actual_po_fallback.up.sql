-- 000599_rm_landed_actual_po_fallback.up.sql
--
-- Extends the ACTUAL section of F_YARN_RM_LANDED's GROUP-RM landed-cost
-- cascade (configured by 000519) with PP as its last-resort fallback:
--   ACTUAL   : CL -> SL -> FL        becomes   CL -> SL -> FL -> PP
--   FORECAST : SP -> PP -> FP        (unchanged)
--   SELLING  : SP -> PP -> FP        (unchanged, still the 000519 placeholder)
--
-- Why: PP is the PO-based landed projection. It mirrors PR, which already
-- closes RM_RATE's ACTUAL cascade (CR -> SR -> PR). Without it, RM groups
-- that only have PO data got RM_LANDED_COST ACTUAL = 0 silently while their
-- RM_RATE resolved a non-zero PR. Evidence: prod period 202608 had 188 of
-- 378 RM groups with CL/SL/FL all zero, so RM_LANDED_COST ACTUAL was 0 and
-- the downstream conversion collapsed to 0 (e.g. POY on RM group 202410800).
--
-- costcalc.ParseRMLandedOrder (loader.go) accepts PP for ACTUAL from the same
-- release, and DefaultRMLandedOrderActual was changed to CL,SL,FL,PP so the
-- hardcoded fallback and this config agree. SP/FP remain invalid for ACTUAL.
--
-- Idempotent: a plain UPDATE to a fixed value; re-running is a no-op apart
-- from updated_at/updated_by. formula_type stays RM_LOOKUP (set by 000519).
UPDATE mst_formula
SET expression = 'ACTUAL:CL,SL,FL,PP;FORECAST:SP,PP,FP;SELLING:SP,PP,FP',
    updated_at = NOW(),
    updated_by = 'migration_000599'
WHERE formula_code = 'F_YARN_RM_LANDED';
