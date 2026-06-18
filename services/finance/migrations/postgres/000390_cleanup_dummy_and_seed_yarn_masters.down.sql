-- 000390 DOWN: Remove seeded MB Head/Spin/BoxBobbin data and re-inserted terminal formula.
-- Note: does NOT restore deleted dummy params/formulas — those were junk data.

BEGIN;

-- Remove F_YARN_STAGE_OUT re-inserted by this migration
DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE formula_code = 'F_YARN_STAGE_OUT' AND created_by = 'seed_000382'
);
UPDATE mst_formula
SET deleted_at = NOW(), deleted_by = 'migration_000390_down'
WHERE formula_code = 'F_YARN_STAGE_OUT' AND created_by = 'seed_000382' AND deleted_at IS NULL;

-- Remove seeded Box/Bobbin Cost
UPDATE mst_box_bobbin_cost
SET deleted_at = NOW(), deleted_by = 'migration_000390_down'
WHERE created_by = 'seed_000390' AND deleted_at IS NULL;

-- Remove seeded MB Spin
UPDATE mst_mb_spin
SET deleted_at = NOW(), deleted_by = 'migration_000390_down'
WHERE created_by = 'seed_000390' AND deleted_at IS NULL;

-- Remove seeded MB Head
UPDATE mst_mb_head
SET deleted_at = NOW(), deleted_by = 'migration_000390_down'
WHERE created_by = 'seed_000390' AND deleted_at IS NULL;

COMMIT;
