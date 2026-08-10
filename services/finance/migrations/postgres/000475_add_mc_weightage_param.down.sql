-- Reverse 000475. Children first (CPP values, CAPP checklist), then the param.
-- Everything is keyed on the 000475 marker, so no pre-existing row is touched.
-- The mst_lookup_master_column row is NOT removed — it predates this migration
-- (000425) and is not ours to drop.

BEGIN;

DELETE FROM cost_product_parameter
WHERE cpp_created_by = 'seed_mc_weightage_000475';

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_mc_weightage_000475';

DELETE FROM mst_parameter
WHERE created_by = 'seed_mc_weightage_000475';

COMMIT;
