-- 000475: MC_WEIGHTAGE parameter — per-machine multiplier for the POY spin
-- fixed-cost pool allocation (see 000474 and 000476).
--
-- Legacy: CST_MST_MACHINE.CMM_WEIGHTAGE, the terminal multiplier in
-- PkgFormulaYarn.fPoyPower_87/88/89/90.
--
-- STATE BEFORE THIS MIGRATION (verified against the live DB):
--   ✓ mst_machine.mc_weightage        exists (000423)
--   ✓ mst_lookup_master_column row    exists (000425, MACHINE / mc_weightage)
--   ✗ mst_parameter row               MISSING  <- added here
--   ✗ lookup_fill_group_code wiring   MISSING  <- added here
--   ✗ per-product CAPP / CPP rows     MISSING  <- added here
--   ✗ Go reader in machineNumericReaders (yarn_lookup_fill_handler.go) — code change,
--     not a migration. 000425 registered the column without its reader, so the
--     fill-group UI could offer it but the handler could not resolve it.
--
-- display_group 'Machine': slots 14 (MC_NAME) .. 17 (MC_EFFICIENCY) are taken;
-- 18 is free and keeps the machine-derived params contiguous.

BEGIN;

-- ============================================================
-- PART 1: The parameter
-- ============================================================
-- Category INPUT (not CALCULATED): the value is filled from the machine master
-- at form-fill time, exactly like MC_EFFICIENCY / POWER_PER_DAY.

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    display_group, display_order, is_active, created_at, created_by
)
SELECT 'MC_WEIGHTAGE', 'Machine Weightage', 'Machine Weightage', 'NUMBER', 'INPUT',
       'Machine', 18, TRUE, NOW(), 'seed_mc_weightage_000475'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'MC_WEIGHTAGE' AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Fill-group wiring (mirrors 000407 PART 3)
-- ============================================================

UPDATE mst_parameter
SET lookup_fill_group_code = 'MC_NAME',
    lookup_source_column   = 'mc_weightage',
    updated_at             = NOW(),
    updated_by             = 'seed_mc_weightage_000475'
WHERE param_code = 'MC_WEIGHTAGE' AND deleted_at IS NULL;

-- ============================================================
-- PART 3: CAPP — checklist MC_WEIGHTAGE for POY (HOY) products only
-- ============================================================
-- Scope is deliberately narrow: only the spin (HOY) track uses the pool model.
-- Adding it elsewhere would put an unused param on 9,426 non-POY cost sheets.

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT pm.cpm_product_sys_id, np.id, FALSE, NULL::INT, 'seed_mc_weightage_000475'
FROM cost_product_master pm
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
                         AND pt.cpt_type_code = 'HOY'
CROSS JOIN mst_parameter np
WHERE np.param_code = 'MC_WEIGHTAGE'
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = pm.cpm_product_sys_id
        AND capp.capp_param_id       = np.id
  );

-- ============================================================
-- PART 4: CPP — backfill the actual value from the machine master
-- ============================================================
-- The fill-group only runs when a user re-saves the form. Existing products
-- need the value now, resolved through their already-filled MC_NAME.
--
-- COALESCE(mc_weightage, 1.0): a NULL weightage is "not configured", and as a
-- terminal multiplier it would silently zero the whole fixed-cost block.
-- An EXPLICIT 0.0000 is respected as Finance data and left as-is.
-- (Live check at authoring time: all 4,003 HOY products resolve to a machine
--  with a non-null, non-zero weightage — this guard is defence for later edits.)

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric,
    cpp_filled_at, cpp_filled_by, cpp_created_at, cpp_created_by
)
SELECT src.cpp_product_sys_id,
       np.id,
       COALESCE(m.mc_weightage, 1.0),
       NOW(), 'seed_mc_weightage_000475', NOW(), 'seed_mc_weightage_000475'
FROM cost_product_parameter src
JOIN mst_parameter sp ON sp.id = src.cpp_param_id
                     AND sp.param_code = 'MC_NAME'
                     AND sp.deleted_at IS NULL
JOIN cost_product_master pm ON pm.cpm_product_sys_id = src.cpp_product_sys_id
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
                         AND pt.cpt_type_code = 'HOY'
LEFT JOIN mst_machine m ON m.mc_name = src.cpp_value_text AND m.deleted_at IS NULL
CROSS JOIN mst_parameter np
WHERE np.param_code = 'MC_WEIGHTAGE'
  AND np.deleted_at IS NULL
  AND src.cpp_value_text IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp2
      WHERE cpp2.cpp_product_sys_id = src.cpp_product_sys_id
        AND cpp2.cpp_param_id       = np.id
  );

COMMIT;

-- ============================================================
-- POST-APPLY VERIFICATION (run manually, not part of the migration)
-- ============================================================
-- Machines whose weightage is explicitly 0 — these zero the POY fixed cost by
-- design. Confirm with Finance that each is intentional:
--
--   SELECT DISTINCT m.mc_name, m.mc_weightage, count(*) AS products
--   FROM cost_product_parameter cpp
--   JOIN mst_parameter p ON p.id = cpp.cpp_param_id AND p.param_code = 'MC_NAME'
--   JOIN cost_product_master pm ON pm.cpm_product_sys_id = cpp.cpp_product_sys_id
--   JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
--                            AND pt.cpt_type_code = 'HOY'
--   JOIN mst_machine m ON m.mc_name = cpp.cpp_value_text AND m.deleted_at IS NULL
--   WHERE m.mc_weightage = 0
--   GROUP BY 1, 2;
--
-- Products that fell back to the 1.0 default (machine row missing or NULL):
--
--   SELECT count(*) FROM cost_product_parameter cpp
--   JOIN mst_parameter p ON p.id = cpp.cpp_param_id AND p.param_code = 'MC_NAME'
--   JOIN cost_product_master pm ON pm.cpm_product_sys_id = cpp.cpp_product_sys_id
--   JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
--                            AND pt.cpt_type_code = 'HOY'
--   LEFT JOIN mst_machine m ON m.mc_name = cpp.cpp_value_text AND m.deleted_at IS NULL
--   WHERE m.mc_weightage IS NULL;
