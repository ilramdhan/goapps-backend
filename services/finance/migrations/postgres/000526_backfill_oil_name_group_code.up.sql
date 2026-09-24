-- 000526 (oil-cost-rm-group M7) — backfill OIL_NAME to RM group codes.
--
-- OIL_NAME is now a MASTER_LOOKUP over RM_GROUP_OIL (000523) storing the RM
-- group code. Existing values are free text (~13,434 rows in prod, D3). For
-- every product whose type has an oil class (000521):
--   * an existing OIL_NAME value that is NOT one of the type's allowed group
--     codes is replaced by the type's DEFAULT group code;
--   * a product whose CAPP has OIL_NAME but has no cost_product_parameter row
--     gets one holding the default group code.
-- Values already holding an allowed code are left untouched. Products of types
-- WITHOUT an oil class are never touched (open item O-6); their count is
-- reported. A type with an oil class but no default mapping is skipped (the
-- engine then BLOCKS those products with MISSING_RM_COST, D13).
--
-- EXACT REVERSIBILITY: every row touched is first copied to
-- bak_oil_name_000526 (old text / numeric / flag / filled_* / updated_*, and
-- had_row = FALSE for inserted rows). The .down.sql restores from it.
-- Marker: cpp_filled_by / cpp_updated_by / cpp_created_by = 'migration_000526'.
-- Zero literal product_sys_id. Idempotent: a second run finds every oil-class
-- value already allowed, backs up nothing new, and changes nothing.

BEGIN;

CREATE TABLE IF NOT EXISTS bak_oil_name_000526 (
    product_sys_id    BIGINT       NOT NULL,
    param_id          UUID         NOT NULL,
    had_row           BOOLEAN      NOT NULL,
    old_value_text    TEXT,
    old_value_numeric NUMERIC(20,6),
    old_value_flag    BOOLEAN,
    old_filled_at     TIMESTAMPTZ,
    old_filled_by     VARCHAR(100),
    old_updated_at    TIMESTAMPTZ,
    old_updated_by    VARCHAR(100),
    backed_up_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (product_sys_id, param_id)
);

COMMENT ON TABLE bak_oil_name_000526 IS
    'Backup written by migration 000526 (OIL_NAME -> RM group code backfill); read by its down migration. Safe to drop once 000526 is final.';

-- Per-product target: the type default + the allowed set (active oil groups only).
CREATE TEMP TABLE tmp_oil_target_000526 ON COMMIT DROP AS
SELECT pm.cpm_product_sys_id                                            AS product_sys_id,
       MAX(gh.group_code) FILTER (WHERE og.cptog_is_default)             AS default_code,
       ARRAY_AGG(gh.group_code) FILTER (WHERE gh.group_code IS NOT NULL) AS allowed_codes
FROM cost_product_master pm
JOIN cost_product_type pt
  ON pt.cpt_type_id = pm.cpm_product_type_id
 AND pt.cpt_oil_class IS NOT NULL
JOIN cost_product_type_oil_group og ON og.cptog_type_id = pt.cpt_type_id
JOIN cst_rm_group_head gh
  ON gh.group_head_id = og.cptog_group_head_id
 AND gh.deleted_at IS NULL
 AND gh.is_oil_group
GROUP BY pm.cpm_product_sys_id;

-- ============================================================
-- PART 1: back up existing rows that will be rewritten
-- ============================================================
INSERT INTO bak_oil_name_000526 (
    product_sys_id, param_id, had_row,
    old_value_text, old_value_numeric, old_value_flag,
    old_filled_at, old_filled_by, old_updated_at, old_updated_by
)
SELECT cpp.cpp_product_sys_id, cpp.cpp_param_id, TRUE,
       cpp.cpp_value_text, cpp.cpp_value_numeric, cpp.cpp_value_flag,
       cpp.cpp_filled_at, cpp.cpp_filled_by, cpp.cpp_updated_at, cpp.cpp_updated_by
FROM cost_product_parameter cpp
JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000526 t ON t.product_sys_id = cpp.cpp_product_sys_id
WHERE t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes))
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

-- ============================================================
-- PART 2: rewrite disallowed values to the type default
-- ============================================================
UPDATE cost_product_parameter cpp
SET cpp_value_text    = t.default_code,
    cpp_value_numeric = NULL,
    cpp_value_flag    = NULL,
    cpp_filled_at     = NOW(),
    cpp_filled_by     = 'migration_000526',
    cpp_updated_at    = NOW(),
    cpp_updated_by    = 'migration_000526'
FROM mst_parameter mp, tmp_oil_target_000526 t
WHERE mp.id = cpp.cpp_param_id
  AND mp.param_code = 'OIL_NAME'
  AND mp.deleted_at IS NULL
  AND t.product_sys_id = cpp.cpp_product_sys_id
  AND t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes));

-- ============================================================
-- PART 3: insert missing OIL_NAME rows where CAPP has OIL_NAME
-- ============================================================
INSERT INTO bak_oil_name_000526 (product_sys_id, param_id, had_row)
SELECT c.capp_product_sys_id, c.capp_param_id, FALSE
FROM cost_product_applicable_param c
JOIN mst_parameter mp ON mp.id = c.capp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000526 t ON t.product_sys_id = c.capp_product_sys_id
WHERE t.default_code IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = c.capp_product_sys_id
        AND cpp.cpp_param_id = c.capp_param_id
  )
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_text,
    cpp_filled_by, cpp_created_by
)
SELECT c.capp_product_sys_id, c.capp_param_id, t.default_code,
       'migration_000526', 'migration_000526'
FROM cost_product_applicable_param c
JOIN mst_parameter mp ON mp.id = c.capp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000526 t ON t.product_sys_id = c.capp_product_sys_id
WHERE t.default_code IS NOT NULL
ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

-- ============================================================
-- Report
-- ============================================================
DO $$
DECLARE
    v_updated   BIGINT;
    v_inserted  BIGINT;
    v_nodefault BIGINT;
    v_nonoil    BIGINT;
BEGIN
    SELECT COUNT(*) FILTER (WHERE had_row), COUNT(*) FILTER (WHERE NOT had_row)
      INTO v_updated, v_inserted
      FROM bak_oil_name_000526;

    SELECT COUNT(*) INTO v_nodefault
    FROM cost_product_master pm
    JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_oil_class IS NOT NULL
    LEFT JOIN tmp_oil_target_000526 t ON t.product_sys_id = pm.cpm_product_sys_id
    WHERE t.default_code IS NULL;

    SELECT COUNT(*) INTO v_nonoil
    FROM cost_product_parameter cpp
    JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
    JOIN cost_product_master pm ON pm.cpm_product_sys_id = cpp.cpp_product_sys_id
    JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id
    WHERE pt.cpt_oil_class IS NULL;

    RAISE NOTICE '000526: OIL_NAME rows updated to type default = %, inserted = %', v_updated, v_inserted;
    RAISE NOTICE '000526: oil-class products skipped (no default mapping) = %', v_nodefault;
    RAISE NOTICE '000526: non-oil-type OIL_NAME rows left untouched = %', v_nonoil;
END $$;

COMMIT;
