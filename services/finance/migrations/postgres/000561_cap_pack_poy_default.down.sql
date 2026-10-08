-- 000561 down — restore the 000408 F_YARN_CAP_PACK and remove what 000561 added.
-- Edge is deleted FIRST, while the marker still identifies the rewritten formula.

BEGIN;

DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id = p.id
  AND f.deleted_at IS NULL
  AND f.updated_by = 'cap_pack_poy_000561'
  AND f.formula_code = 'F_YARN_CAP_PACK'
  AND p.param_code = 'CAP_PACK_POY_DEFAULT';

UPDATE mst_formula
SET expression  = 'CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0',
    description = 'Packing cost captive',
    updated_at  = NULL,
    updated_by  = NULL
WHERE formula_code = 'F_YARN_CAP_PACK'
  AND deleted_at IS NULL
  AND updated_by = 'cap_pack_poy_000561'
  AND expression = 'IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)';

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_cap_pack_poy_000561';

DELETE FROM formula_param fp
USING mst_formula f
WHERE fp.formula_id = f.id
  AND f.formula_code = 'F_YARN_CAP_PACK_POY_DEFAULT'
  AND f.created_by = 'seed_000561'
  AND f.deleted_at IS NULL;

UPDATE mst_formula
SET deleted_at = NOW(),
    deleted_by = 'migration_000561_down'
WHERE formula_code = 'F_YARN_CAP_PACK_POY_DEFAULT'
  AND created_by = 'seed_000561'
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(),
    deleted_by = 'migration_000561_down'
WHERE param_code = 'CAP_PACK_POY_DEFAULT'
  AND created_by = 'seed_000561'
  AND deleted_at IS NULL;

COMMIT;
