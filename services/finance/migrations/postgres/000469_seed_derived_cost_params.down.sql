-- Reverse 000469. Order matters: CAPP rows and formula_param links reference the
-- formulas/params, so they go first. Everything is keyed on the 000469 marker,
-- so no pre-existing row is touched.

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_derived_000469';

DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE created_by = 'seed_derived_000469'
);

DELETE FROM mst_formula   WHERE created_by = 'seed_derived_000469';
DELETE FROM mst_parameter WHERE created_by = 'seed_derived_000469';

COMMIT;
