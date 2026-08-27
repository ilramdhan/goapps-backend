-- 000480 rollback.
--
-- Order matters: mst_formula.result_param_id FKs to mst_parameter(id), so the
-- 3 formula rows must go before the 3 virtual params they point at.
--
-- ⚠ DESTRUCTIVE: DROP TABLE removes the 2 seeded factor rows and any pair a user
-- later added through the master UI. The formula/param DELETEs are hard deletes
-- (not soft) — this matches how the up migration created them, and mirrors
-- 000452's rollback style.

BEGIN;

-- 1. Formula rows first (they reference the virtual params).
DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula
    WHERE formula_code IN ('F_MB_LDR_SCALE','F_MB_LDR_XSECTION','F_MB_LDR_STRENGTH')
);

DELETE FROM mst_formula
WHERE formula_code IN ('F_MB_LDR_SCALE','F_MB_LDR_XSECTION','F_MB_LDR_STRENGTH');

-- 2. Then the virtual result params.
DELETE FROM mst_parameter
WHERE param_code IN ('MB_LDR_SCALE','MB_LDR_XSECTION','MB_LDR_STRENGTH');

-- 3. Restore the formula_type CHECK to its 12-value form (state left by 000451).
--    Must run AFTER the F_MB_LDR_XSECTION row is gone, otherwise the ADD
--    CONSTRAINT fails validating that surviving row.
ALTER TABLE mst_formula DROP CONSTRAINT IF EXISTS mst_formula_formula_type_check;
ALTER TABLE mst_formula ADD CONSTRAINT mst_formula_formula_type_check
  CHECK (formula_type IN (
    'CALCULATION','SQL_QUERY','CONSTANT','LOOKUP','RM_LOOKUP','CONDITIONAL',
    'FROM_MARKETING','INTERMINGLING','SNAPSHOT','PENDING','INITIAL_VALUE',
    'MB_COST_LOOKUP'
  ));

-- 4. Finally the factor table (its FKs into mst_mb_cross_section go with it).
DROP TABLE IF EXISTS mst_mb_cross_section_factor;

COMMIT;
