-- 000519_configure_yarn_rm_landed_cascade.down.sql
--
-- Restores F_YARN_RM_LANDED to its exact original 000408 seed values
-- (pass-through CALCULATION of RM_RATE).
UPDATE mst_formula
SET formula_type = 'CALCULATION',
    expression = 'RM_RATE',
    updated_at = NOW(),
    updated_by = 'migration_000519_down'
WHERE formula_code = 'F_YARN_RM_LANDED';
