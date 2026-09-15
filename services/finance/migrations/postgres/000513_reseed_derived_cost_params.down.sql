-- Reverse 000513 (the re-seed of 000469's data).
--
-- Order is the exact inverse of the up file: CAPP rows and formula_param edges
-- reference the formulas/params, so they are removed first, then the formulas,
-- then the params.
--
-- Scope: ONLY rows carrying the 'reseed_derived_000513' marker. Rows written by a
-- genuine 000469 run (marker 'seed_derived_000469') are never touched, and neither
-- is anything pre-existing.
--
-- KNOWN LIMITATION (structural, not an oversight): formula_param has NO audit
-- columns — see 000005_create_mst_formula.up.sql lines 46-51, the table is only
-- (id, formula_id, param_id, sort_order). Edges can therefore only be attributed
-- through their owning formula. If 000513 ran against a database where the
-- formulas already existed under the 000469 marker and only the edges were
-- missing, those edges were inserted by 000513 but are owned by a 000469 formula,
-- so this down file deliberately leaves them in place rather than deleting rows it
-- cannot prove it created. Running 000469's own down file removes them.

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'reseed_derived_000513';

DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE created_by = 'reseed_derived_000513'
);

DELETE FROM mst_formula   WHERE created_by = 'reseed_derived_000513';
DELETE FROM mst_parameter WHERE created_by = 'reseed_derived_000513';

COMMIT;
