-- 000245_backfill_default_capp_values.up.sql
--
-- For every (product, applicable INPUT param) pair where cost_product_parameter
-- has no value yet, insert a sensible default. Engine BLOCKED when formulas
-- reference NULL inputs — this gives every product a working baseline.
--
-- Defaults are per-stage agnostic — values are deliberately conservative.
-- Users can override per-product in the Parameters UI afterwards.

BEGIN;

WITH defaults AS (
    SELECT * FROM (VALUES
        ('YARN_DENIER',       150.0),
        ('DYESTUFF_OWF_PCT',  4.0),
        ('CONES_PER_KG',      0.8),
        ('MACHINE_RPM',       2500.0),
        ('INSPECT_HR_TON',    9.0),
        ('INT_TRANSPORT',     500.0),
        ('PACK_LBR_HR_TON',   3.5),
        ('STEAM_KG_DYE',      4.2),
        ('WATER_L_PER_KG',    120.0),
        ('GAS_NM3_PER_KG',    0.05),
        ('GAS_RATE_M3',       8000.0),
        ('CHILL_PER_KG',      0.02),
        ('CHILL_RATE',        2500.0),
        ('LBR_RATE_TECH',     35000.0),
        ('STEAM_KG',          2.5),
        ('STEAM_RATE',        800.0),
        ('WATER_M3',          0.12),
        ('WATER_RATE',        12000.0),
        ('LABOR_HRS',         3.5),
        ('LABOR_RATE',        22000.0),
        ('ELEC_KWH',          1.2),
        ('ELEC_RATE',         1500.0),
        ('WASTE_PCT',         2.5),
        ('YIELD_PCT',         96.0),
        ('MARGIN_PCT',        18.0),
        ('MAT_OVERHEAD_PCT',  8.0),
        ('LABOR_OVERHEAD_PCT', 25.0),
        ('DEPREC_PER_KG',     450.0),
        ('MAINT_PER_KG',      280.0),
        ('FACTORY_OH',        900.0),
        ('QC_PER_KG',         120.0),
        ('PACK_PER_KG',       250.0),
        ('AIR_PER_KG',        200.0)
    ) AS d(param_code, default_value)
)
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric, cpp_value_text,
    cpp_filled_by, cpp_created_by
)
SELECT capp.capp_product_sys_id,
       capp.capp_param_id,
       d.default_value,
       NULL,
       'seed_000245_backfill',
       'seed_000245_backfill'
FROM cost_product_applicable_param capp
JOIN mst_parameter mp ON mp.id = capp.capp_param_id
JOIN defaults d ON d.param_code = mp.param_code
WHERE mp.param_category = 'INPUT'
  AND mp.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = capp.capp_product_sys_id
        AND cpp.cpp_param_id = capp.capp_param_id
  );

COMMIT;
