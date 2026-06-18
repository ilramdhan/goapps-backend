-- 000391 DOWN: Re-delete restored params and remove F_YARN_STAGE_OUT.

BEGIN;

DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE formula_code = 'F_YARN_STAGE_OUT' AND created_by = 'seed_000382'
);
UPDATE mst_formula
SET deleted_at = NOW(), deleted_by = 'migration_000391_down'
WHERE formula_code = 'F_YARN_STAGE_OUT' AND created_by = 'seed_000382' AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000391_down'
WHERE created_by = 'seed_000381'
  AND param_code IN (
    'COST_CONVERSION', 'COST_ELEC', 'COST_LABOR', 'COST_STAGE_OUT',
    'DEPREC_PER_KG', 'DRAW_RATIO', 'ELEC_KWH', 'ELEC_RATE',
    'FILAMENT_COUNT', 'LABOR_HRS', 'LABOR_OVERHEAD_PCT', 'LABOR_RATE',
    'LUSTRE_TYPE', 'MACHINE_RPM', 'MAT_OVERHEAD_PCT', 'POLYMER_IV'
  )
  AND deleted_at IS NULL;

COMMIT;
