-- 000519_configure_yarn_rm_landed_cascade.up.sql
--
-- Stops F_YARN_RM_LANDED (result_param RM_LANDED_COST, "RM Landed Cost" on
-- product-master) from being a pass-through CALCULATION of RM_RATE
-- (000408_seed_oracle_formulas.up.sql: expression='RM_RATE'). RM_LANDED_COST
-- gets its own GROUP-type RM cascade, distinct from RM_RATE's CR/SR/PR
-- cascade and calc_type-DEPENDENT (unlike RM_RATE's, which is calc_type-
-- agnostic):
--   ACTUAL             -> first non-zero of CL -> SL -> FL
--   FORECAST           -> first non-zero of SP -> PP -> FP
--   SELLING            -> DELIBERATE PLACEHOLDER: no distinct "landed"
--                         simulation value exists anywhere in the codebase
--                         today (SELLING/cost_sim is only meaningful for
--                         ITEM-type RMs, and GROUP-type RM_RATE's own
--                         cascade already bypasses SELLING/cost_sim
--                         entirely). Per explicit product decision, SELLING
--                         is treated identically to FORECAST (SP->PP->FP)
--                         until a real SELLING-specific landed definition
--                         exists. Revisit this row when that definition
--                         lands.
--
-- formula_type flips CALCULATION -> RM_LOOKUP so evalSingleFormulaStep routes
-- it to costcalc's dedicated Go resolver (resolveRMLandedCost, compute.go)
-- instead of the expr-lang evaluator, mirroring the 3 pre-existing RM_LOOKUP
-- formulas (F_YARN_RM_RATE, F_YARN_CAP_CONVERSION, F_YARN_DEL_CONVERSION).
-- RM_LOOKUP formulas are never evaluated by expr-lang (see compute.go's
-- FormulaType switch), so the expression column is display-only/config text
-- here exactly as it already is for F_YARN_RM_RATE (000518) -- repurposing it
-- changes no expr-lang behavior.
--
-- New format contract (parsed by costcalc.ParseRMLandedOrder, loader.go):
-- semicolon-separated per-calc_type sections, each "CALC_TYPE:CSV-of-tokens".
--   Valid calc_type keys : ACTUAL, FORECAST, SELLING
--   Valid tokens (ACTUAL)            : CL, SL, FL
--   Valid tokens (FORECAST, SELLING) : SP, PP, FP
-- A missing/unparseable/invalid section for a given calc_type falls back
-- independently to that calc_type's hardcoded default -- a config typo in one
-- section must never hard-fail (or corrupt) computation for the others.
--
-- The formula_param edge (F_YARN_RM_LANDED -> RM_RATE, sort_order 1, seeded
-- by 000408 line 135) is intentionally left in place, not removed. It is
-- vestigial now that F_YARN_RM_LANDED's result is no longer computed by
-- reading RM_RATE out of scope (loadPerProductFormulas only loads formulas
-- whose result_param has a CAPP row for the product; formula_param merely
-- feeds InputParamCodes, which RM_LOOKUP-typed formulas never consult --
-- see evalSingleFormulaStep's FormulaTypeRMLookup case). Removing it is a
-- UI-display-only change (formula dependency graphs, if any read
-- formula_param) with no computation impact either way, so it is left alone
-- rather than risk breaking a display feature this migration cannot see.
UPDATE mst_formula
SET formula_type = 'RM_LOOKUP',
    expression = 'ACTUAL:CL,SL,FL;FORECAST:SP,PP,FP;SELLING:SP,PP,FP',
    updated_at = NOW(),
    updated_by = 'migration_000519'
WHERE formula_code = 'F_YARN_RM_LANDED';
