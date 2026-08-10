-- 000476: Switch POY (HOY) fixed cost per-kg to the legacy spin-pool model.
--
-- ============================================================
-- WHY THIS SHAPE (read before editing)
-- ============================================================
-- The original plan was "new POY-specific result params + swap the CAPP
-- checklist". That was abandoned after measuring the actual dependency graph:
-- TOTAL_FIXEDCOST_PER_KG feeds an 8-level cascade that reconverges
--
--   TOTAL_FIXEDCOST_PER_KG
--     -> ONLY_CONV_{CAP,DEL}_PACK_EXCL_MB
--       -> {CAPTIVE,DELIVERY}_COST_BEFORE_QLOSS
--         -> BC_VAL_LOSS_*, NON_STD_VALUE_LOSS, {CAPTIVE,DELIVERY}_COST_QLTY_LOSS
--           -> QLTY_LOSS_*, VOLUME_BUCKET_1..5_DEL_COST, DOMESTIC_COST
--             -> DOMESTIC_COST_UNEVEN_PACK
--
-- Cloning result params for POY would require duplicating ~20 params and every
-- formula between them, because idx_mst_formula_result_param_unique allows only
-- one live formula per result param. Any param left un-cloned mid-chain becomes
-- a zero-filled placeholder and SILENTLY zeros the cost with no error.
--
-- So the branch lives INSIDE the 4 leaf formulas instead. Non-POY products take
-- the untouched ELSE arm and are bit-identical to before; only POY takes the
-- pool arm. The 9,426 non-POY products sharing these formula rows are safe.
--
-- ============================================================
-- THE MODEL
-- ============================================================
-- Legacy PkgFormulaYarn.fPoyPower_87 / fPoyManPower_88 / fPoyOverheads_89 /
-- fPoyConsSprs_90 allocate a SHARED monthly pool:
--
--   *_PER_KG = spin_xxx_month / poy_production * common_poy_denier
--                             / ACT_DENIER * mc_weightage
--
-- NOT the bottom-up *_PER_DAY / NET_PRODUCTION the engine used, which inflated
-- POY fixed cost ~8.4x. Note the legacy overhead formula does NOT multiply by
-- NO_OF_END — the engine's ELSE arm keeps it for non-POY, the pool arm drops it.
--
-- SPIN_COMMON_POY_DENIER / SPIN_POY_PRODUCTION / SPIN_POWER_MONTH /
-- SPIN_MANPOWER_MONTH / SPIN_OVERHEADS_MONTH / SPIN_CONSSPRS_MONTH are injected
-- into the evaluation scope by the Go engine from mst_spin_fixed_cost (000474),
-- following the existing COST_RM_TOTAL precedent. They are deliberately NOT
-- mst_parameter rows: they are period-global, not per-product, and must not
-- appear on any product's CAPP form.
--
-- VERIFIED (product 90299 / cpm_flex_02 '83', machine C-10-S w=1.0, denier 250):
--   POWER    198634 / 3027153 * 329.712 / 250 = 0.086539  vs legacy 0.0865  OK
--   MANPOWER 275561 / 3027153 * 329.712 / 250 = 0.120073  vs legacy 0.1201  OK
--   OVERHEAD  46600 / 3027153 * 329.712 / 250 = 0.020305  vs legacy 0.0203  OK
--   SPARES    54100 / 3027153 * 329.712 / 250 = 0.023573  vs legacy 0.0236  OK

BEGIN;

-- ============================================================
-- PART 1: IS_SPIN_POOL_MODEL — the branch discriminator
-- ============================================================
-- A dedicated flag rather than reusing MC_WEIGHTAGE > 0: a machine whose
-- weightage is legitimately 0.0000 would otherwise silently fall back to the
-- wrong (bottom-up) model. This param is auditable in cpc_param_snapshot, so
-- which arm ran is visible per product after every calculation.
--
-- Products without a CAPP row for it zero-fill to 0 -> ELSE arm -> unchanged.
-- display_group 'Machine' slot 19 (18 taken by MC_WEIGHTAGE in 000475).

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, min_value, max_value, display_group, display_order,
    is_active, notes, created_at, created_by
)
SELECT 'IS_SPIN_POOL_MODEL', 'Uses Spin Pool Fixed Cost Model', 'Spin Pool Model',
       'NUMBER', 'INPUT', 0, 0, 1, 'Machine', 19, TRUE,
       '1 = fixed cost per kg allocated from the shared monthly spin pool (POY). '
       '0 = bottom-up per-machine per-day / net production.',
       NOW(), 'seed_spin_pool_000476'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'IS_SPIN_POOL_MODEL' AND deleted_at IS NULL
);

-- CAPP: POY (HOY) products only.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT pm.cpm_product_sys_id, np.id, FALSE, NULL::INT, 'seed_spin_pool_000476'
FROM cost_product_master pm
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
                         AND pt.cpt_type_code = 'HOY'
CROSS JOIN mst_parameter np
WHERE np.param_code = 'IS_SPIN_POOL_MODEL'
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = pm.cpm_product_sys_id
        AND capp.capp_param_id       = np.id
  );

-- CPP: set the flag to 1 for those same products.
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric,
    cpp_filled_at, cpp_filled_by, cpp_created_at, cpp_created_by
)
SELECT DISTINCT pm.cpm_product_sys_id, np.id, 1,
       NOW(), 'seed_spin_pool_000476', NOW(), 'seed_spin_pool_000476'
FROM cost_product_master pm
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
                         AND pt.cpt_type_code = 'HOY'
CROSS JOIN mst_parameter np
WHERE np.param_code = 'IS_SPIN_POOL_MODEL'
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp2
      WHERE cpp2.cpp_product_sys_id = pm.cpm_product_sys_id
        AND cpp2.cpp_param_id       = np.id
  );

-- ============================================================
-- PART 2: Rewrite the 4 leaf formula expressions
-- ============================================================
-- Guards: ACT_DENIER > 0 and SPIN_POY_PRODUCTION > 0 replace legacy's implicit
-- nvl() — Oracle would raise on a zero divisor, the new engine must return 0.
-- The ELSE arm is copied verbatim from 000408 and must stay that way.

UPDATE mst_formula f SET
    expression = 'IS_SPIN_POOL_MODEL > 0 ? (ACT_DENIER > 0 && SPIN_POY_PRODUCTION > 0 ? SPIN_POWER_MONTH / SPIN_POY_PRODUCTION * SPIN_COMMON_POY_DENIER / ACT_DENIER * MC_WEIGHTAGE : 0) : (NET_PRODUCTION > 0 ? POWER_PER_DAY / NET_PRODUCTION : 0)',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'seed_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_POWER_KG' AND f.deleted_at IS NULL;

UPDATE mst_formula f SET
    expression = 'IS_SPIN_POOL_MODEL > 0 ? (ACT_DENIER > 0 && SPIN_POY_PRODUCTION > 0 ? SPIN_MANPOWER_MONTH / SPIN_POY_PRODUCTION * SPIN_COMMON_POY_DENIER / ACT_DENIER * MC_WEIGHTAGE : 0) : (NET_PRODUCTION > 0 ? MANPOWER_PER_DAY / NET_PRODUCTION : 0)',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'seed_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_MANPOWER_KG' AND f.deleted_at IS NULL;

-- NO_OF_END appears only in the ELSE arm — legacy fPoyOverheads_89 does not use it.
UPDATE mst_formula f SET
    expression = 'IS_SPIN_POOL_MODEL > 0 ? (ACT_DENIER > 0 && SPIN_POY_PRODUCTION > 0 ? SPIN_OVERHEADS_MONTH / SPIN_POY_PRODUCTION * SPIN_COMMON_POY_DENIER / ACT_DENIER * MC_WEIGHTAGE : 0) : (NET_PRODUCTION > 0 ? OVERHEAD_PER_HEAD * NO_OF_END / NET_PRODUCTION : 0)',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'seed_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_OVERHEAD_KG' AND f.deleted_at IS NULL;

UPDATE mst_formula f SET
    expression = 'IS_SPIN_POOL_MODEL > 0 ? (ACT_DENIER > 0 && SPIN_POY_PRODUCTION > 0 ? SPIN_CONSSPRS_MONTH / SPIN_POY_PRODUCTION * SPIN_COMMON_POY_DENIER / ACT_DENIER * MC_WEIGHTAGE : 0) : (NET_PRODUCTION > 0 ? SPARESCOST_PER_DAY / NET_PRODUCTION : 0)',
    version    = f.version + 1,
    updated_at = NOW(),
    updated_by = 'seed_spin_pool_000476'
WHERE f.formula_code = 'F_YARN_SPARES_KG' AND f.deleted_at IS NULL;

-- ============================================================
-- PART 3: formula_param — declare the new inputs
-- ============================================================
-- Drives topo-sort, zero-fill and the trace input list. Only mst_parameter-backed
-- codes belong here; the SPIN_* globals are scope-injected by Go (like
-- COST_RM_TOTAL) and are intentionally absent.
--
-- Existing rows (POWER_PER_DAY, NET_PRODUCTION, ...) are kept: the ELSE arm still
-- references them, and dropping them would zero-fill those codes out of the trace.

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
  ('F_YARN_POWER_KG',   'IS_SPIN_POOL_MODEL',10),('F_YARN_POWER_KG',   'ACT_DENIER',11),('F_YARN_POWER_KG',   'MC_WEIGHTAGE',12),
  ('F_YARN_MANPOWER_KG','IS_SPIN_POOL_MODEL',10),('F_YARN_MANPOWER_KG','ACT_DENIER',11),('F_YARN_MANPOWER_KG','MC_WEIGHTAGE',12),
  ('F_YARN_OVERHEAD_KG','IS_SPIN_POOL_MODEL',10),('F_YARN_OVERHEAD_KG','ACT_DENIER',11),('F_YARN_OVERHEAD_KG','MC_WEIGHTAGE',12),
  ('F_YARN_SPARES_KG',  'IS_SPIN_POOL_MODEL',10),('F_YARN_SPARES_KG',  'ACT_DENIER',11),('F_YARN_SPARES_KG',  'MC_WEIGHTAGE',12)
) AS fp(fcode, pcode, sort_order)
WHERE
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND NOT EXISTS (
    SELECT 1 FROM formula_param fp2
    WHERE fp2.formula_id = (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1)
      AND fp2.param_id   = (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1)
);

COMMIT;

-- ============================================================
-- POST-APPLY VERIFICATION (manual)
-- ============================================================
-- Requires the Go scope injection to be deployed first — without it the SPIN_*
-- codes are undefined and every POY fixed cost evaluates to 0.
--
-- After recomputing product 90299 for a period that has an mst_spin_fixed_cost
-- row, cpc_param_snapshot should read:
--     POWER_PER_KG            0.0865
--     MANPOWER_PER_KG         0.1201
--     OVERHEAD_PER_KG         0.0203
--     SPARESCOST_PER_KG       0.0236
--     TOTAL_FIXEDCOST_PER_KG  ~0.522
--     DELIVERY_COST_QLTY_LOSS ~1.3206   (legacy CYCC TOP 118-122)
--
-- Non-POY regression: recompute any TPY product and diff cpc_cost_per_unit
-- against its pre-migration value — it must be byte-identical.
