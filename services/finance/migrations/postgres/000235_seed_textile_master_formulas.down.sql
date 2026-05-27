-- 000235 down: remove textile formulas (and their formula_param rows via CASCADE).
BEGIN;

UPDATE mst_formula
   SET deleted_at = NOW(),
       deleted_by = 'seed_000235_down',
       is_active  = FALSE
 WHERE formula_code IN (
       'F_TEXTILE_STEAM','F_TEXTILE_WATER','F_TEXTILE_UTIL','F_TEXTILE_OVERHEAD',
       'F_TEXTILE_AFTER_YLD','F_TEXTILE_SELLING'
   )
   AND created_by = 'seed_000235'
   AND deleted_at IS NULL;

COMMIT;
