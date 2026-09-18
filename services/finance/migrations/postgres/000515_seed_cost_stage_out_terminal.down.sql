-- Reverse 000515 (the re-seed of the COST_STAGE_OUT terminal sink).
--
-- Order is the exact inverse of the up file: CAPP rows and the formula_param edge
-- reference the formula/param, so they go first, then the formula, then the param.
--
-- Scope: ONLY rows carrying the 'seed_stage_out_000515' marker. Anything pre-existing
-- — in particular any row a historical 000381 / 000390 / 000391 run may have left
-- behind under marker 'seed_000381' / 'seed_000382' — is never touched.
--
-- KNOWN LIMITATION (structural, not an oversight): formula_param has NO audit columns
-- at all — see 000005_create_mst_formula.up.sql lines 46-51, the table is only
-- (id, formula_id, param_id, sort_order). An edge can therefore only be attributed
-- through its OWNING FORMULA. If 000515 ran against a database where F_YARN_STAGE_OUT
-- already existed under a different marker and only the edge was missing, that edge
-- was inserted by 000515 but is owned by someone else's formula, so this file
-- deliberately leaves it in place rather than deleting a row it cannot prove it wrote.
--
-- AFTER RUNNING THIS: branch (a) of compute.go:212 goes dead again and cpc_cost_per_unit
-- reverts to the findTerminalFormula heuristic (depth + FormulaCode ASC). Already
-- persisted cst_product_cost values are NOT rewritten by this file; a recalculation is
-- required for the reversal to show up in stored costs.

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_stage_out_000515';

DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE created_by = 'seed_stage_out_000515'
);

DELETE FROM mst_formula   WHERE created_by = 'seed_stage_out_000515';
DELETE FROM mst_parameter WHERE created_by = 'seed_stage_out_000515';

COMMIT;
