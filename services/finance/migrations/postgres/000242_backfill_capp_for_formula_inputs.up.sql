-- 000242_backfill_capp_for_formula_inputs.up.sql
--
-- Backfill cost_product_applicable_param (CAPP) rows so EVERY product has
-- every input param referenced by any active formula automatically checklisted.
--
-- Why: user-flagged regression — products created before the formula library
-- expansion (000234-000241) lack CAPP entries for the new INPUT params. The
-- calc engine compute step needs CAPP set so the param shows in the per-product
-- Parameters tab + gets included in the formula scope.
--
-- Strategy: idempotent INSERT ... SELECT WHERE NOT EXISTS, deriving the matrix
-- from (active products) × (active-formula input params). capp_param_id is
-- already unique per product so ON CONFLICT (capp_product_sys_id, capp_param_id)
-- DO NOTHING also works — using NOT EXISTS for symmetry with other seeds.

BEGIN;

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT
    cpm.cpm_product_sys_id,
    fp.param_id,
    FALSE                       AS capp_is_required,
    NULL::INT                   AS capp_display_order,
    'seed_000242_backfill'      AS capp_created_by
FROM cost_product_master cpm
CROSS JOIN mst_formula f
JOIN formula_param fp ON fp.formula_id = f.id
WHERE f.is_active = TRUE
  AND f.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = cpm.cpm_product_sys_id
        AND capp.capp_param_id      = fp.param_id
  );

-- Also checklist the result_param of each active formula (CALCULATED sink)
-- so it appears in the per-product Parameters tab as "auto-populated by formula".
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT
    cpm.cpm_product_sys_id,
    f.result_param_id,
    FALSE                       AS capp_is_required,
    NULL::INT                   AS capp_display_order,
    'seed_000242_backfill'      AS capp_created_by
FROM cost_product_master cpm
CROSS JOIN mst_formula f
WHERE f.is_active = TRUE
  AND f.deleted_at IS NULL
  AND f.result_param_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = cpm.cpm_product_sys_id
        AND capp.capp_param_id      = f.result_param_id
  );

COMMIT;
