-- 000570 (rm-ratio-denier) — denier-based RM ratio for MEL / ACY products.
--
-- 1. cost_product_type.cpt_rm_ratio_mode ('INPUT' | 'DENIER', default 'INPUT').
--    DENIER = the calc engine derives the RM line ratios of the product from
--    denier (upstream DENIER / RM group head denier / SPANDEX_DRAFT) instead of
--    the route RM ratio. Seeded 'DENIER' for MEL and ACY.
-- 2. cst_rm_group_head.denier: nominal denier of an RM group (e.g. spandex
--    20/40/70). Physical spec — HEAD-LEVEL and GLOBAL, deliberately NOT added
--    to cst_rm_group_head_period (000503), mirroring is_oil_group (000520).
--    Left NULL; values are entered by users.
-- 3. mst_parameter SPANDEX_DRAFT ("Spandex Draft Ratio", INPUT, NUMBER) in the
--    'Yarn Specification' display group, right after the denier params.
-- 4. For every ACY product: CAPP row for SPANDEX_DRAFT (not required) and a
--    cost_product_parameter value of 3 — inserted only where absent, an
--    existing value is never overwritten.
--
-- All seeding joins on CODES only — zero literal ids. On a database without
-- MEL/ACY types or ACY products the seeds are no-ops.
--
-- Idempotent: ADD COLUMN IF NOT EXISTS, DO-block guarded ADD CONSTRAINT,
-- WHERE NOT EXISTS / ON CONFLICT DO NOTHING inserts, guarded UPDATE.

BEGIN;

-- ============================================================
-- PART 1: cost_product_type.cpt_rm_ratio_mode
-- ============================================================
ALTER TABLE cost_product_type
    ADD COLUMN IF NOT EXISTS cpt_rm_ratio_mode VARCHAR(10) NOT NULL DEFAULT 'INPUT';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_cpt_rm_ratio_mode'
    ) THEN
        ALTER TABLE cost_product_type
            ADD CONSTRAINT chk_cpt_rm_ratio_mode
            CHECK (cpt_rm_ratio_mode IN ('INPUT', 'DENIER'));
    END IF;
END $$;

COMMENT ON COLUMN cost_product_type.cpt_rm_ratio_mode IS
    'RM ratio mode: INPUT = route RM ratio as entered; DENIER = ratio derived from denier (upstream DENIER, RM group head denier / SPANDEX_DRAFT).';

UPDATE cost_product_type
SET cpt_rm_ratio_mode = 'DENIER',
    cpt_updated_at    = NOW()
WHERE cpt_type_code IN ('MEL', 'ACY')
  AND cpt_rm_ratio_mode <> 'DENIER';

-- ============================================================
-- PART 2: cst_rm_group_head.denier
-- ============================================================
ALTER TABLE cst_rm_group_head
    ADD COLUMN IF NOT EXISTS denier NUMERIC(12,4);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_denier_positive'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_denier_positive
            CHECK (denier IS NULL OR denier > 0);
    END IF;
END $$;

COMMENT ON COLUMN cst_rm_group_head.denier IS
    'Nominal denier, e.g. spandex 20/40/70; spandex denier in DENIER-ratio products = denier / SPANDEX_DRAFT. Head-level/global, not period-versioned.';

-- ============================================================
-- PART 3: mst_parameter SPANDEX_DRAFT
-- ============================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, is_active,
    display_group, display_order, notes,
    created_at, created_by
)
SELECT 'SPANDEX_DRAFT', 'Spandex Draft Ratio', 'Spd Draft', 'NUMBER', 'INPUT',
       NULL, NULL, 0, 100, TRUE,
       'Yarn Specification', 26,
       'Spandex draft ratio (e.g. 3 / 3.3). Spandex denier in DENIER-ratio products = RM group denier / SPANDEX_DRAFT.',
       NOW(), 'migration_000570'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter
    WHERE param_code = 'SPANDEX_DRAFT' AND deleted_at IS NULL
);

-- ============================================================
-- PART 4: ACY products — CAPP + value 3 (only where absent)
-- ============================================================
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by
)
SELECT m.cpm_product_sys_id, mp.id, FALSE, 'migration_000570'
FROM cost_product_master m
JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
JOIN mst_parameter mp    ON mp.param_code = 'SPANDEX_DRAFT' AND mp.deleted_at IS NULL
WHERE t.cpt_type_code = 'ACY'
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric,
    cpp_filled_by, cpp_created_by
)
SELECT m.cpm_product_sys_id, mp.id, 3,
       'migration_000570', 'migration_000570'
FROM cost_product_master m
JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
JOIN mst_parameter mp    ON mp.param_code = 'SPANDEX_DRAFT' AND mp.deleted_at IS NULL
WHERE t.cpt_type_code = 'ACY'
ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

DO $$
DECLARE
    v_types INT;
    v_capp  INT;
    v_vals  INT;
BEGIN
    SELECT COUNT(*) INTO v_types FROM cost_product_type WHERE cpt_rm_ratio_mode = 'DENIER';
    SELECT COUNT(*) INTO v_capp
    FROM cost_product_applicable_param c
    JOIN mst_parameter mp ON mp.id = c.capp_param_id
    WHERE mp.param_code = 'SPANDEX_DRAFT';
    SELECT COUNT(*) INTO v_vals
    FROM cost_product_parameter c
    JOIN mst_parameter mp ON mp.id = c.cpp_param_id
    WHERE mp.param_code = 'SPANDEX_DRAFT';
    RAISE NOTICE '000570: DENIER product types = % (expected 2), SPANDEX_DRAFT capp rows = %, values = %', v_types, v_capp, v_vals;
END $$;

COMMIT;
