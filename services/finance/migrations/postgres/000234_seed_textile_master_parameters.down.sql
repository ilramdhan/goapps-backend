-- 000234 down: remove textile master parameters seeded in the up migration.
-- Soft-deletes (sets deleted_at) to preserve any cost_product_parameter /
-- cost_product_applicable_param / formula_param FK rows that might already
-- reference these. Hard delete is intentionally avoided.

BEGIN;

UPDATE mst_parameter
   SET deleted_at = NOW(),
       deleted_by = 'seed_000234_down',
       is_active  = FALSE
 WHERE param_code IN (
       'YIELD_PCT','SHRINK_PCT','OIL_PICKUP_PCT','IND_LABOR_PCT',
       'MACHINE_PER_KG','STEAM_KG','STEAM_RATE','WATER_M3','WATER_RATE','AIR_PER_KG',
       'MAINT_PER_KG','FACTORY_OH','QC_PER_KG','PACK_PER_KG','MARGIN_PCT',
       'COST_STEAM','COST_WATER','COST_UTIL','COST_OVERHEAD','COST_AFTER_YLD','SELLING_PRICE'
   )
   AND created_by = 'seed_000234'
   AND deleted_at IS NULL;

COMMIT;
