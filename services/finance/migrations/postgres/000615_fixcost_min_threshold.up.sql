-- 000615 (fixcost-min-threshold): minimum fixed cost per kg floor for PLY/TFO.
--
-- Finance: on PLY / TFO machines the fixed cost per kg must never fall below
-- 0.05. The engine floors TOTAL_FIXEDCOST_PER_KG at a threshold and stores the
-- gap in the engine-injected param FIXEDCOST_SHORTFALL, so downstream
-- conversion / domestic cost include it automatically.
--
-- Threshold resolution (engine): product cpm_fixcost_min_threshold if set
-- (0 = floor off), else the machine's mc_fixcost_min_threshold, else no floor.
--
-- PART 1: nullable columns + CHECK (>= 0). NULL = auto / no floor.
-- PART 2: seed the 5 PLY/TFO machines with 0.05 (skipped when missing or already set).
-- PART 3: FIXEDCOST_SHORTFALL param (engine-injected; category INPUT like
--         IS_SPIN_POOL_MODEL, 000476). NO formula, NO formula_param and NO
--         cost_product_applicable_param row: a CALCULATED/CAPP param without a
--         formula would trip rejectCalculatedParamsWithoutFormula.
-- PART 4: backfill active products whose MC_NAME text matches a machine that
--         has a threshold. updated_at of products is deliberately NOT touched
--         (a config backfill, not a user edit).
-- Marker 'fixcost_min_threshold_000615' (created_by / updated_by); down deletes
-- the param row by that marker. Idempotent: every step is a no-op on re-run.
-- Machine match (same as 000614): trimmed, case-insensitive mc_name OR mc_code.

BEGIN;

SET LOCAL lock_timeout = '10s';

-- PART 1 ---------------------------------------------------------------------
ALTER TABLE cost_product_master
    ADD COLUMN IF NOT EXISTS cpm_fixcost_min_threshold NUMERIC(20,6) NULL;

ALTER TABLE mst_machine
    ADD COLUMN IF NOT EXISTS mc_fixcost_min_threshold NUMERIC(20,6) NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_cpm_fixcost_min_threshold'
    ) THEN
        ALTER TABLE cost_product_master
            ADD CONSTRAINT chk_cpm_fixcost_min_threshold
            CHECK (cpm_fixcost_min_threshold IS NULL OR cpm_fixcost_min_threshold >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_mc_fixcost_min_threshold'
    ) THEN
        ALTER TABLE mst_machine
            ADD CONSTRAINT chk_mc_fixcost_min_threshold
            CHECK (mc_fixcost_min_threshold IS NULL OR mc_fixcost_min_threshold >= 0);
    END IF;
END $$;

COMMENT ON COLUMN cost_product_master.cpm_fixcost_min_threshold IS
    'Fixed cost floor per product (000615): NULL = auto (machine default), 0 = floor off, >0 = floor per kg on TOTAL_FIXEDCOST_PER_KG';

COMMENT ON COLUMN mst_machine.mc_fixcost_min_threshold IS
    'Machine default fixed cost floor per kg (000615): NULL = no floor, 0 = off, >0 = floor on TOTAL_FIXEDCOST_PER_KG for products on this machine unless the product overrides';

-- PART 2 ---------------------------------------------------------------------
UPDATE mst_machine
   SET mc_fixcost_min_threshold = 0.05,
       updated_at = NOW(),
       updated_by = 'fixcost_min_threshold_000615'
 WHERE deleted_at IS NULL
   AND mc_fixcost_min_threshold IS NULL
   AND (LOWER(BTRIM(mc_name)) IN ('ply', 'ply ( carp)', 'ply 30-poss', 'tfo (lumino)', '50% poss tfo (lumino)')
        OR LOWER(BTRIM(mc_code)) IN ('ply', 'ply ( carp)', 'ply 30-poss', 'tfo (lumino)', '50% poss tfo (lumino)'));

-- PART 3 ---------------------------------------------------------------------
-- Same display group as TOTAL_FIXEDCOST_PER_KG ('Fixed Cost', order 91, 000429);
-- 92 is free inside that group. uom copied from TOTAL_FIXEDCOST_PER_KG.
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, display_group, display_order,
    is_active, notes, created_at, created_by
)
SELECT 'FIXEDCOST_SHORTFALL', 'Fixed Cost Shortfall to Min Threshold', 'FixCost Shortfall',
       'NUMBER', 'INPUT',
       (SELECT t.uom_id FROM mst_parameter t
         WHERE t.param_code = 'TOTAL_FIXEDCOST_PER_KG' AND t.deleted_at IS NULL),
       0, 'Fixed Cost', 92, TRUE,
       'Engine-injected: min threshold minus TOTAL_FIXEDCOST_PER_KG when the floor applies, else 0. Not attached to CAPP, no formula.',
       NOW(), 'fixcost_min_threshold_000615'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'FIXEDCOST_SHORTFALL' AND deleted_at IS NULL
);

-- PART 4 ---------------------------------------------------------------------
UPDATE cost_product_master pm
   SET cpm_fixcost_min_threshold = 0.05
 WHERE pm.cpm_is_active
   AND pm.cpm_fixcost_min_threshold IS NULL
   AND EXISTS (
       SELECT 1
       FROM cost_product_parameter cpp
       JOIN mst_parameter mc
         ON mc.id = cpp.cpp_param_id
        AND mc.param_code = 'MC_NAME'
        AND mc.deleted_at IS NULL
       JOIN mst_machine m
         ON m.deleted_at IS NULL
        AND m.mc_fixcost_min_threshold IS NOT NULL
        AND (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))
             OR LOWER(BTRIM(m.mc_code)) = LOWER(BTRIM(cpp.cpp_value_text)))
       WHERE cpp.cpp_product_sys_id = pm.cpm_product_sys_id
         AND NULLIF(BTRIM(cpp.cpp_value_text), '') IS NOT NULL
   );

COMMIT;
