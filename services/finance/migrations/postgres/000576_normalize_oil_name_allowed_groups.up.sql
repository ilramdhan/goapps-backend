-- 000576 (master-lookup-sync follow-up) — normalize OIL_NAME to allowed oil groups.
--
-- Prod check 2026-10-16: 33 oil-class products hold an OIL_NAME that is not an
-- allowed oil-group code for their product type, so the calc engine BLOCKS
-- them with MISSING_RM_COST (loader_oil.go / oil_rate.go: btrim(OIL_NAME) must
-- be a group_code of an active, is_oil_group cst_rm_group_head mapped to the
-- type in cost_product_type_oil_group):
--   * MEL 16: 'CONING OIL' (free text) x12, 'MGTC 14' x4 — ex-TCY/ATY products
--     moved to MEL by 000572 AFTER 000526 had normalized OIL_NAME.
--   * PTY 16: 'MGTC 14' x12, 'CONOL TF 1638' x1, '202006077' (SPIN FINISH OIL) x3
--     — written with cpp_filled_by = 'migration' on 2026-09-29 (CSTPTY2609*).
--   * TTS 1: '202006101' (CONING OIL).
--   'MGTC 14' / 'CONOL TF 1638' are not RM groups at all.
--
-- PART 1 (user decision 2026-10-16): extra ALLOWED, non-default mappings
--   PTY <- 202006077 SPIN FINISH OIL
--   TTS <- 202006101 CONING OIL
-- added only when the type exists and the group exists, is not soft-deleted
-- and is_oil_group. Marker cptog_created_by = 'migration_000576'. The ids of
-- the rows actually inserted go to bak_oil_group_map_000576 so the down
-- migration removes only those.
--
-- PART 2: the 000526 rule re-applied to every oil-class type, against the
-- allowed set computed AFTER part 1: an existing OIL_NAME row whose
-- btrim(text) is not an allowed code is rewritten to the type DEFAULT code.
-- The old row is backed up first in bak_oil_name_000576 (same columns as
-- bak_oil_name_000526). Marker cpp_filled_by / cpp_updated_by =
-- 'migration_000576'. Types with no default mapping are skipped and reported.
-- Missing OIL_NAME rows are NOT inserted (calc falls back to the default).
--
-- Expected prod effect: MEL 16 -> 202006101; PTY 13 ('MGTC 14' 12,
-- 'CONOL TF 1638' 1) -> PTY default (202006101); PTY 3 '202006077' and TTS 1
-- '202006101' stay (now allowed). Recalculate those products after deploy.
--
-- Zero literal product / type / group ids. Idempotent: mapping INSERT ON
-- CONFLICT DO NOTHING, backup INSERT ON CONFLICT DO NOTHING (keeps the first
-- original value), a second run finds every value allowed and changes nothing.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS bak_oil_group_map_000576 (
    cptog_id     INT          PRIMARY KEY,
    backed_up_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE bak_oil_group_map_000576 IS
    'Oil-group mapping rows inserted by migration 000576; read by its down migration. Safe to drop once 000576 is final.';

CREATE TABLE IF NOT EXISTS bak_oil_name_000576 (
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

COMMENT ON TABLE bak_oil_name_000576 IS
    'Backup written by migration 000576 (OIL_NAME -> allowed oil group normalization); read by its down migration. Safe to drop once 000576 is final.';

-- ============================================================
-- PART 1: extra allowed (non-default) oil-group mappings
-- ============================================================
WITH ins AS (
    INSERT INTO cost_product_type_oil_group (
        cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by
    )
    SELECT t.cpt_type_id, g.group_head_id, FALSE, 'migration_000576'
    FROM (VALUES
        ('PTY', '202006077'),
        ('TTS', '202006101')
    ) AS v(type_code, group_code)
    JOIN cost_product_type t ON t.cpt_type_code = v.type_code
    JOIN cst_rm_group_head g
      ON g.group_code = v.group_code
     AND g.deleted_at IS NULL
     AND g.is_oil_group
    ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING
    RETURNING cptog_id
)
INSERT INTO bak_oil_group_map_000576 (cptog_id)
SELECT cptog_id FROM ins
ON CONFLICT (cptog_id) DO NOTHING;

-- Per-product target: the type default + the allowed set (active oil groups
-- only), computed after part 1.
CREATE TEMP TABLE tmp_oil_target_000576 ON COMMIT DROP AS
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
-- PART 2a: back up existing rows that will be rewritten
-- ============================================================
INSERT INTO bak_oil_name_000576 (
    product_sys_id, param_id, had_row,
    old_value_text, old_value_numeric, old_value_flag,
    old_filled_at, old_filled_by, old_updated_at, old_updated_by
)
SELECT cpp.cpp_product_sys_id, cpp.cpp_param_id, TRUE,
       cpp.cpp_value_text, cpp.cpp_value_numeric, cpp.cpp_value_flag,
       cpp.cpp_filled_at, cpp.cpp_filled_by, cpp.cpp_updated_at, cpp.cpp_updated_by
FROM cost_product_parameter cpp
JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000576 t ON t.product_sys_id = cpp.cpp_product_sys_id
WHERE t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes))
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

-- ============================================================
-- PART 2b: rewrite disallowed values to the type default
-- ============================================================
UPDATE cost_product_parameter cpp
SET cpp_value_text    = t.default_code,
    cpp_value_numeric = NULL,
    cpp_value_flag    = NULL,
    cpp_filled_at     = NOW(),
    cpp_filled_by     = 'migration_000576',
    cpp_updated_at    = NOW(),
    cpp_updated_by    = 'migration_000576'
FROM mst_parameter mp, tmp_oil_target_000576 t
WHERE mp.id = cpp.cpp_param_id
  AND mp.param_code = 'OIL_NAME'
  AND mp.deleted_at IS NULL
  AND t.product_sys_id = cpp.cpp_product_sys_id
  AND t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes));

-- ============================================================
-- Report
-- ============================================================
DO $$
DECLARE
    v_mappings  BIGINT;
    v_updated   BIGINT;
    v_nodefault BIGINT;
    v_invalid   BIGINT;
BEGIN
    SELECT COUNT(*) INTO v_mappings FROM bak_oil_group_map_000576;
    SELECT COUNT(*) INTO v_updated  FROM bak_oil_name_000576;

    SELECT COUNT(*) INTO v_nodefault
    FROM cost_product_master pm
    JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_oil_class IS NOT NULL
    LEFT JOIN tmp_oil_target_000576 t ON t.product_sys_id = pm.cpm_product_sys_id
    WHERE t.default_code IS NULL;

    SELECT COUNT(*) INTO v_invalid
    FROM cost_product_parameter cpp
    JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
    JOIN tmp_oil_target_000576 t ON t.product_sys_id = cpp.cpp_product_sys_id
    WHERE NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes));

    RAISE NOTICE '000576: oil-group mappings inserted = %', v_mappings;
    RAISE NOTICE '000576: OIL_NAME rows rewritten to type default = %', v_updated;
    RAISE NOTICE '000576: oil-class products skipped (no default mapping) = %', v_nodefault;
    RAISE NOTICE '000576: OIL_NAME rows still not allowed (type without default) = %', v_invalid;
END $$;

COMMIT;
