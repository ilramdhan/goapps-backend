-- 000577 (master-lookup-sync follow-up) — oil class PTY for ATY and ACY.
--
-- Finance rule (2026-10-16): POY, PTY, ATY, ACY (Aalidra) carry oil cost and
-- oil gain. ATY and ACY had no oil class, so no oil formulas ran for them.
--
-- 1. cost_product_type ATY / ACY: cpt_oil_class = 'PTY' only where NULL. The
--    previous value is kept in bak_oil_class_000577 for the down migration.
-- 2. PTY's oil-group mapping (allowed groups + default CONING OIL 202006101;
--    after 000576 also 202006077) is copied to ATY and ACY. A copied default
--    is only inserted when the type has no default yet (uk_cptog_type_default).
--    Marker cptog_created_by = 'migration_000577'; inserted ids go to
--    bak_oil_group_map_000577.
-- 3. Products of ATY / ACY with OPU > 0 get the missing oil CAPP rows (the 7
--    params of 000572: OIL_NAME, OIL_RATE, OIL_COST, OIL_GAIN,
--    OIL_GAIN_POY_DEFAULT, OPU, WASTE_PERC), marker capp_created_by =
--    'migration_000577'. Products with OPU 0 / no OPU get NO CAPP rows.
-- 4. OIL_NAME normalized for every ATY / ACY product with an OIL_NAME row: a
--    value (btrim) not in the type's allowed codes -> type default code. Old
--    rows are backed up in bak_oil_name_000577 (same shape as 000576).
--    OPU > 0 products whose CAPP has OIL_NAME but no cpp row get one with the
--    default (backup had_row = FALSE). Markers cpp_filled_by / cpp_updated_by
--    = 'migration_000577'.
--
-- Expected prod effect: ACY 106 (OPU > 0: 3 = CSTACY2606000042/069/087),
-- ATY 372 (OPU > 0: 342). Most ATY OIL_NAME are text 'CONING OIL' ->
-- 202006101. OPU 0 products get the oil class but the engine skips oil when
-- OPU <= 0 (separate engine commit). Recalculate all ATY / ACY after deploy.
--
-- Zero literal ids. Idempotent: class UPDATE only on NULL, INSERTs ON CONFLICT
-- DO NOTHING, backups keep the first original value, a second run finds
-- everything allowed and changes nothing.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS bak_oil_class_000577 (
    type_code     VARCHAR(50)  PRIMARY KEY,
    old_oil_class VARCHAR(50),
    backed_up_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bak_oil_group_map_000577 (
    cptog_id     INT          PRIMARY KEY,
    backed_up_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bak_oil_name_000577 (
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

COMMENT ON TABLE bak_oil_class_000577 IS
    'Previous cpt_oil_class of ATY/ACY before migration 000577; read by its down migration. Safe to drop once 000577 is final.';
COMMENT ON TABLE bak_oil_group_map_000577 IS
    'Oil-group mapping rows inserted by migration 000577; read by its down migration. Safe to drop once 000577 is final.';
COMMENT ON TABLE bak_oil_name_000577 IS
    'Backup written by migration 000577 (ATY/ACY OIL_NAME normalization); read by its down migration. Safe to drop once 000577 is final.';

-- ============================================================
-- PART 1: oil class PTY for ATY / ACY (only when NULL)
-- ============================================================
INSERT INTO bak_oil_class_000577 (type_code, old_oil_class)
SELECT t.cpt_type_code, t.cpt_oil_class
FROM cost_product_type t
WHERE t.cpt_type_code IN ('ATY', 'ACY')
  AND t.cpt_oil_class IS NULL
  AND (SELECT pty.cpt_oil_class FROM cost_product_type pty WHERE pty.cpt_type_code = 'PTY') IS NOT NULL
ON CONFLICT (type_code) DO NOTHING;

UPDATE cost_product_type t
SET cpt_oil_class  = (SELECT pty.cpt_oil_class FROM cost_product_type pty WHERE pty.cpt_type_code = 'PTY'),
    cpt_updated_at = NOW()
WHERE t.cpt_type_code IN ('ATY', 'ACY')
  AND t.cpt_oil_class IS NULL
  AND (SELECT pty.cpt_oil_class FROM cost_product_type pty WHERE pty.cpt_type_code = 'PTY') IS NOT NULL;

-- ============================================================
-- PART 2: copy PTY oil-group mapping to ATY / ACY
-- ============================================================
WITH ins AS (
    INSERT INTO cost_product_type_oil_group (
        cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by
    )
    SELECT dst.cpt_type_id, g.cptog_group_head_id, g.cptog_is_default, 'migration_000577'
    FROM cost_product_type_oil_group g
    JOIN cost_product_type pty ON pty.cpt_type_id = g.cptog_type_id AND pty.cpt_type_code = 'PTY'
    CROSS JOIN cost_product_type dst
    WHERE dst.cpt_type_code IN ('ATY', 'ACY')
      AND (g.cptog_is_default = FALSE OR NOT EXISTS (
          SELECT 1 FROM cost_product_type_oil_group d
          WHERE d.cptog_type_id = dst.cpt_type_id AND d.cptog_is_default
      ))
    ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING
    RETURNING cptog_id
)
INSERT INTO bak_oil_group_map_000577 (cptog_id)
SELECT cptog_id FROM ins
ON CONFLICT (cptog_id) DO NOTHING;

-- ============================================================
-- PART 3: oil CAPP rows for ATY / ACY products with OPU > 0
-- ============================================================
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT pm.cpm_product_sys_id, mp.id, FALSE, NULL::INT, 'migration_000577'
FROM cost_product_master pm
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_type_code IN ('ATY', 'ACY')
CROSS JOIN mst_parameter mp
WHERE mp.deleted_at IS NULL
  AND mp.param_code IN ('OIL_NAME', 'OIL_RATE', 'OIL_COST', 'OIL_GAIN',
                        'OIL_GAIN_POY_DEFAULT', 'OPU', 'WASTE_PERC')
  AND EXISTS (
      SELECT 1
      FROM cost_product_parameter cpp
      JOIN mst_parameter op ON op.id = cpp.cpp_param_id AND op.param_code = 'OPU' AND op.deleted_at IS NULL
      WHERE cpp.cpp_product_sys_id = pm.cpm_product_sys_id
        AND cpp.cpp_value_numeric > 0
  )
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

-- ============================================================
-- PART 4: OIL_NAME normalization (allowed set computed after part 2)
-- ============================================================
CREATE TEMP TABLE tmp_oil_target_000577 ON COMMIT DROP AS
SELECT pm.cpm_product_sys_id                                            AS product_sys_id,
       MAX(gh.group_code) FILTER (WHERE og.cptog_is_default)             AS default_code,
       ARRAY_AGG(gh.group_code) FILTER (WHERE gh.group_code IS NOT NULL) AS allowed_codes
FROM cost_product_master pm
JOIN cost_product_type pt
  ON pt.cpt_type_id = pm.cpm_product_type_id
 AND pt.cpt_type_code IN ('ATY', 'ACY')
 AND pt.cpt_oil_class IS NOT NULL
JOIN cost_product_type_oil_group og ON og.cptog_type_id = pt.cpt_type_id
JOIN cst_rm_group_head gh
  ON gh.group_head_id = og.cptog_group_head_id
 AND gh.deleted_at IS NULL
 AND gh.is_oil_group
GROUP BY pm.cpm_product_sys_id;

-- 4a: back up rows that will be rewritten
INSERT INTO bak_oil_name_000577 (
    product_sys_id, param_id, had_row,
    old_value_text, old_value_numeric, old_value_flag,
    old_filled_at, old_filled_by, old_updated_at, old_updated_by
)
SELECT cpp.cpp_product_sys_id, cpp.cpp_param_id, TRUE,
       cpp.cpp_value_text, cpp.cpp_value_numeric, cpp.cpp_value_flag,
       cpp.cpp_filled_at, cpp.cpp_filled_by, cpp.cpp_updated_at, cpp.cpp_updated_by
FROM cost_product_parameter cpp
JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000577 t ON t.product_sys_id = cpp.cpp_product_sys_id
WHERE t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes))
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

-- 4b: rewrite disallowed values to the type default
UPDATE cost_product_parameter cpp
SET cpp_value_text    = t.default_code,
    cpp_value_numeric = NULL,
    cpp_value_flag    = NULL,
    cpp_filled_at     = NOW(),
    cpp_filled_by     = 'migration_000577',
    cpp_updated_at    = NOW(),
    cpp_updated_by    = 'migration_000577'
FROM mst_parameter mp, tmp_oil_target_000577 t
WHERE mp.id = cpp.cpp_param_id
  AND mp.param_code = 'OIL_NAME'
  AND mp.deleted_at IS NULL
  AND t.product_sys_id = cpp.cpp_product_sys_id
  AND t.default_code IS NOT NULL
  AND NOT (COALESCE(btrim(cpp.cpp_value_text), '') = ANY (t.allowed_codes));

-- 4c: OPU > 0 products with OIL_NAME in CAPP but no value row -> default
INSERT INTO bak_oil_name_000577 (product_sys_id, param_id, had_row)
SELECT capp.capp_product_sys_id, mp.id, FALSE
FROM cost_product_applicable_param capp
JOIN mst_parameter mp ON mp.id = capp.capp_param_id AND mp.param_code = 'OIL_NAME' AND mp.deleted_at IS NULL
JOIN tmp_oil_target_000577 t ON t.product_sys_id = capp.capp_product_sys_id
WHERE t.default_code IS NOT NULL
  AND EXISTS (
      SELECT 1
      FROM cost_product_parameter o
      JOIN mst_parameter op ON op.id = o.cpp_param_id AND op.param_code = 'OPU' AND op.deleted_at IS NULL
      WHERE o.cpp_product_sys_id = capp.capp_product_sys_id AND o.cpp_value_numeric > 0
  )
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter x
      WHERE x.cpp_product_sys_id = capp.capp_product_sys_id AND x.cpp_param_id = mp.id
  )
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_text,
    cpp_filled_at, cpp_filled_by, cpp_created_by, cpp_updated_at, cpp_updated_by
)
SELECT b.product_sys_id, b.param_id, t.default_code,
       NOW(), 'migration_000577', 'migration_000577', NOW(), 'migration_000577'
FROM bak_oil_name_000577 b
JOIN tmp_oil_target_000577 t ON t.product_sys_id = b.product_sys_id
WHERE NOT b.had_row
  AND t.default_code IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter x
      WHERE x.cpp_product_sys_id = b.product_sys_id AND x.cpp_param_id = b.param_id
  );

-- ============================================================
-- Report
-- ============================================================
DO $$
DECLARE
    v_classes  BIGINT;
    v_maps     BIGINT;
    v_capp     BIGINT;
    v_rewrite  BIGINT;
    v_inserted BIGINT;
    v_nodef    BIGINT;
BEGIN
    SELECT COUNT(*) INTO v_classes FROM bak_oil_class_000577;
    SELECT COUNT(*) INTO v_maps FROM bak_oil_group_map_000577;
    SELECT COUNT(*) INTO v_capp FROM cost_product_applicable_param WHERE capp_created_by = 'migration_000577';
    SELECT COUNT(*) INTO v_rewrite FROM bak_oil_name_000577 WHERE had_row;
    SELECT COUNT(*) INTO v_inserted FROM bak_oil_name_000577 WHERE NOT had_row;
    SELECT COUNT(*) INTO v_nodef
    FROM cost_product_master pm
    JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_type_code IN ('ATY', 'ACY')
    LEFT JOIN tmp_oil_target_000577 t ON t.product_sys_id = pm.cpm_product_sys_id
    WHERE t.default_code IS NULL;

    RAISE NOTICE '000577: ATY/ACY oil class set to PTY on % type(s)', v_classes;
    RAISE NOTICE '000577: oil-group mappings inserted = %', v_maps;
    RAISE NOTICE '000577: oil CAPP rows added = %', v_capp;
    RAISE NOTICE '000577: OIL_NAME rows rewritten to type default = %, rows inserted = %', v_rewrite, v_inserted;
    RAISE NOTICE '000577: ATY/ACY products skipped (no default mapping) = %', v_nodef;
END $$;

COMMIT;
