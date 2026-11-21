-- 000614 down: remove only what 000614 created (marker
-- 'machine_param_default_000614'), then drop the table. A value a user saved
-- afterwards is KEPT: the upsert overwrites cpp_filled_by / cpp_updated_by, so
-- only untouched backfill rows (marker in created_by AND filled_by, never
-- updated) are deleted. CAPP rows are kept when the product still has a value for that
-- param, so a kept value is never orphaned.

BEGIN;

DELETE FROM cost_product_parameter
WHERE cpp_created_by = 'machine_param_default_000614'
  AND cpp_filled_by = 'machine_param_default_000614'
  AND cpp_updated_by IS NULL;

DELETE FROM cost_product_applicable_param a
WHERE a.capp_created_by = 'machine_param_default_000614'
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = a.capp_product_sys_id
        AND cpp.cpp_param_id = a.capp_param_id);

DROP TABLE IF EXISTS mst_machine_param_default;

COMMIT;
