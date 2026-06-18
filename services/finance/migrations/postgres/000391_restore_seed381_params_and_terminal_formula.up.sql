-- 000391: Restore seed_000381 canonical params incorrectly deleted by migration_000390.
-- Context: 000390 soft-deleted ALL 'seed' params but 16 of them are in seed_000381 VALUES.
--          seed_000381 had skipped those 16 because old 'seed' already held them (NOT EXISTS guard).
--          Deleting them left a hole: COST_STAGE_OUT gone → F_YARN_STAGE_OUT could not be inserted.
-- Fix:
--   1. Un-delete the 16 'seed' params that seed_000381 defines — re-attribute to seed_000381.
--      (NOT COST_RM_LOADED / COST_RM_TOTAL / COST_LABOR_FULL — those are 'seed'-only junk.)
--   2. Insert F_YARN_STAGE_OUT terminal formula + its formula_param row.

BEGIN;

-- ─── 1. Restore 16 canonical params from 'seed' creator ──────────────────────
UPDATE mst_parameter
SET deleted_at = NULL,
    deleted_by = NULL,
    created_by = 'seed_000381'
WHERE created_by = 'seed'
  AND deleted_at IS NOT NULL
  AND param_code IN (
    'COST_CONVERSION',
    'COST_ELEC',
    'COST_LABOR',
    'COST_STAGE_OUT',
    'DEPREC_PER_KG',
    'DRAW_RATIO',
    'ELEC_KWH',
    'ELEC_RATE',
    'FILAMENT_COUNT',
    'LABOR_HRS',
    'LABOR_OVERHEAD_PCT',
    'LABOR_RATE',
    'LUSTRE_TYPE',
    'MACHINE_RPM',
    'MAT_OVERHEAD_PCT',
    'POLYMER_IV'
  );

-- ─── 2. Insert F_YARN_STAGE_OUT terminal formula ─────────────────────────────
INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, created_by
)
SELECT
    'F_YARN_STAGE_OUT',
    'Terminal engine sink',
    'CALCULATION',
    'COST_DEL_FINAL',
    p.id,
    'Passthrough: COST_STAGE_OUT = COST_DEL_FINAL. Required by ScopeKeyFinalCost.',
    'seed_000382'
FROM mst_parameter p
WHERE p.param_code = 'COST_STAGE_OUT'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula f
      WHERE f.formula_code = 'F_YARN_STAGE_OUT' AND f.deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula fchk
      WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL
  );

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 0
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'COST_DEL_FINAL' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_STAGE_OUT'
  AND f.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp
      WHERE fp.formula_id = f.id AND fp.param_id = p.id
  );

COMMIT;
