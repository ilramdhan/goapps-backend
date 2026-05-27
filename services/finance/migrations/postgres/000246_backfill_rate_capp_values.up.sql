-- 000246_backfill_rate_capp_values.up.sql
--
-- Follow-up to 000245: that migration filtered param_category='INPUT' and
-- left RATE params un-valued. F_TX_INSPECT_QC references LBR_RATE_TECH
-- (RATE) which stayed NULL → engine ErrFormulaEval at first chip-level
-- compute → cascading MISSING_UPSTREAM_COST for all 17 downstream products.
--
-- Same pattern as 000245 but extended to RATE category.

BEGIN;

WITH defaults AS (
    SELECT * FROM (VALUES
        ('LBR_RATE_TECH',  35000.0),
        ('GAS_RATE_M3',     8000.0),
        ('GAS_NM3_PER_KG',     0.05),
        ('CHILL_PER_KG',       0.02),
        ('CHILL_RATE',      2500.0),
        ('WATER_RATE',     12000.0),
        ('STEAM_RATE',       800.0),
        ('ELEC_RATE',       1500.0),
        ('LABOR_RATE',     22000.0),
        ('CONDENSATE_PCT',    85.0)
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
       'seed_000246_backfill',
       'seed_000246_backfill'
FROM cost_product_applicable_param capp
JOIN mst_parameter mp ON mp.id = capp.capp_param_id
JOIN defaults d ON d.param_code = mp.param_code
WHERE mp.param_category IN ('RATE', 'INPUT')
  AND mp.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = capp.capp_product_sys_id
        AND cpp.cpp_param_id = capp.capp_param_id
  );

COMMIT;
