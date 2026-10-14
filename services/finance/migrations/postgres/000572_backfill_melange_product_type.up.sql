-- 000572 (rm-group-denier-draft-flag P2b) — backfill melange products to type
-- MEL and give MEL the PTY oil class.
--
-- 174 melange products (finance list Melange_Product_List.xlsx, key = legacy
-- sys id cost_product_master.cpm_flex_02; prod check 2026-10-14: 174/174 found,
-- 158 PTY + 15 TCY + 1 ATY, none MEL yet) are moved to product type MEL.
--
-- 1. cost_product_type MEL gets PTY's cpt_oil_class (only when NULL).
-- 2. PTY's oil-group mapping (cost_product_type_oil_group: allowed groups and
--    the default, CONING OIL 202006101) is copied to MEL. Without it
--    resolveOilRate would block every MEL product (no default / not allowed).
-- 3. cost_product_master.cpm_product_type_id -> MEL for the 174 legacy ids,
--    plus missing oil CAPP rows for them (see below).
--
-- BEHAVIOUR CHANGE (user decision 2026-10-14, intended):
--   * All 174 products are MEL with oil class PTY, so IS_PTY = 1 for them.
--   * The 158 former PTY products keep their oil cost/gain behaviour.
--   * The 15 former TCY products and the 1 former ATY product
--     (CSTATY2606000035) had NO oil class; OIL_COST / OIL_GAIN are now
--     calculated for them too. If their CAPP lacks OIL_NAME the type default
--     group is used; if OPU is missing OIL_COST is 0 (no error).
--   * The 7 oil params (OIL_NAME, OIL_RATE, OIL_COST, OIL_GAIN,
--     OIL_GAIN_POY_DEFAULT, OPU, WASTE_PERC) are added to the CAPP of every
--     moved product that lacks them (000525 pattern), marker
--     capp_created_by = 'migration_000572'; without them the oil formulas are
--     not run for the former TCY/ATY products.
--   * TX Weight config: MEL already shares the DTY config (000537), like PTY
--     and ATY; former TCY products move from the TTY config to the DTY one.
--   * MEL is DENIER rm_ratio_mode (000570): RM ratios of these products are
--     now derived from denier share instead of the route ratio.
--   Recalculate the affected products after deploy.
--
-- Reversibility: previous type per product is stored in
-- cost_product_type_backfill_572; the MEL oil-class change is recorded in
-- cost_product_type_backfill_572_meta. The down migration restores from both.
-- Mapping rows carry cptog_created_by = 'migration_000572'.
--
-- Idempotent: class UPDATE only on NULL, mapping INSERT ON CONFLICT DO NOTHING,
-- backup INSERT ON CONFLICT DO NOTHING (keeps the first old type), product
-- UPDATE only where the type is not yet MEL. Zero literal UUIDs / type ids.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS cost_product_type_backfill_572 (
    cpm_product_sys_id BIGINT      PRIMARY KEY,
    old_type_id        INT         NOT NULL,
    backed_up_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS cost_product_type_backfill_572_meta (
    key   VARCHAR(50) PRIMARY KEY,
    value TEXT
);

COMMENT ON TABLE cost_product_type_backfill_572 IS
    'Backup written by migration 000572 (melange -> MEL backfill): previous product type per product; read by its down migration. Safe to drop once 000572 is final.';
COMMENT ON TABLE cost_product_type_backfill_572_meta IS
    'Markers written by migration 000572 (e.g. MEL oil class set by it); read by its down migration.';

CREATE TEMP TABLE tmp_melange_572 (legacy_id VARCHAR(20) PRIMARY KEY) ON COMMIT DROP;
INSERT INTO tmp_melange_572 (legacy_id)
SELECT unnest(ARRAY[
        '6972', '6957', '8015', '10051', '7137', '7150', '12172', '9443', '5184', '7190', '2919', '2840',
        '7621', '7965', '4576', '5356', '8167', '7031', '8464', '8465', '8466', '6691', '6947', '6320',
        '2769', '2776', '2785', '2829', '2837', '2838', '2839', '2841', '3616', '5349', '5350', '5351',
        '5352', '5353', '5354', '5355', '2669', '8183', '6756', '7185', '7494', '6485', '2719', '3630',
        '5828', '3621', '11677', '3512', '12597', '1673', '1675', '1314', '1307', '5831', '6275', '12950',
        '12952', '12953', '8401', '1630', '1608', '11086', '11010', '1611', '1610', '11281', '3050', '3072',
        '3093', '3094', '12414', '1609', '1607', '3098', '11089', '4869', '4899', '1612', '7846', '7848',
        '11039', '12134', '1313', '1321', '1666', '12823', '6651', '6652', '6653', '6654', '6655', '6656',
        '10759', '10761', '11524', '9102', '9103', '5427', '10211', '13476', '6997', '7249', '4360', '2206',
        '2209', '3388', '4862', '4866', '4876', '80', '11525', '11526', '11527', '11533', '8776', '8940',
        '8942', '8944', '8945', '8947', '8946', '8943', '8941', '4091', '10831', '10832', '10833', '4893',
        '4892', '9138', '8555', '12222', '11090', '12001', '10203', '10202', '10201', '10200', '10199', '3686',
        '3681', '3676', '3675', '2208', '2207', '2307', '11997', '11996', '11995', '11994', '11532', '9259',
        '3688', '3685', '3680', '3677', '2140', '12000', '11999', '11998', '11993', '8685', '8684', '8683',
        '3687', '3683', '3682', '3678', '3673', '2657'
]::VARCHAR(20)[]);

-- ============================================================
-- PART 1: MEL oil class = PTY oil class (only when NULL)
-- ============================================================
WITH upd AS (
    UPDATE cost_product_type mel
    SET cpt_oil_class  = (SELECT pty.cpt_oil_class FROM cost_product_type pty WHERE pty.cpt_type_code = 'PTY'),
        cpt_updated_at = NOW()
    WHERE mel.cpt_type_code = 'MEL'
      AND mel.cpt_oil_class IS NULL
      AND (SELECT pty.cpt_oil_class FROM cost_product_type pty WHERE pty.cpt_type_code = 'PTY') IS NOT NULL
    RETURNING mel.cpt_type_id
)
INSERT INTO cost_product_type_backfill_572_meta (key, value)
SELECT 'mel_oil_class_set', 'true' FROM upd
ON CONFLICT (key) DO NOTHING;

-- ============================================================
-- PART 2: copy PTY oil-group mapping to MEL
-- ============================================================
INSERT INTO cost_product_type_oil_group (
    cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by
)
SELECT mel.cpt_type_id, g.cptog_group_head_id, g.cptog_is_default, 'migration_000572'
FROM cost_product_type_oil_group g
JOIN cost_product_type pty ON pty.cpt_type_id = g.cptog_type_id AND pty.cpt_type_code = 'PTY'
CROSS JOIN cost_product_type mel
WHERE mel.cpt_type_code = 'MEL'
  AND (g.cptog_is_default = FALSE OR NOT EXISTS (
      SELECT 1 FROM cost_product_type_oil_group d
      WHERE d.cptog_type_id = mel.cpt_type_id AND d.cptog_is_default
  ))
ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING;

-- ============================================================
-- PART 3: move the 174 melange products to MEL (backup first)
-- ============================================================
INSERT INTO cost_product_type_backfill_572 (cpm_product_sys_id, old_type_id)
SELECT pm.cpm_product_sys_id, pm.cpm_product_type_id
FROM cost_product_master pm
JOIN tmp_melange_572 k ON k.legacy_id = pm.cpm_flex_02
JOIN cost_product_type mel ON mel.cpt_type_code = 'MEL'
WHERE pm.cpm_product_type_id <> mel.cpt_type_id
ON CONFLICT (cpm_product_sys_id) DO NOTHING;

UPDATE cost_product_master pm
SET cpm_product_type_id = mel.cpt_type_id,
    cpm_updated_at      = NOW(),
    cpm_updated_by      = 'migration_000572'
FROM tmp_melange_572 k, cost_product_type mel
WHERE k.legacy_id = pm.cpm_flex_02
  AND mel.cpt_type_code = 'MEL'
  AND pm.cpm_product_type_id <> mel.cpt_type_id;

-- ============================================================
-- PART 3b: oil CAPP rows for the moved products (000525 pattern)
-- ============================================================
-- LoadFormulas only runs a formula whose result param is in the product's
-- CAPP. 000525 seeded the 7 oil params only for products of oil-class types,
-- so former TCY/ATY products lack them and their OIL_COST / OIL_GAIN would
-- silently not be calculated. Rows that already exist are left untouched.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT pm.cpm_product_sys_id, mp.id, FALSE, NULL::INT, 'migration_000572'
FROM cost_product_master pm
JOIN tmp_melange_572 k ON k.legacy_id = pm.cpm_flex_02
JOIN cost_product_type mel ON mel.cpt_type_id = pm.cpm_product_type_id AND mel.cpt_type_code = 'MEL'
CROSS JOIN mst_parameter mp
WHERE mp.deleted_at IS NULL
  AND mp.param_code IN ('OIL_NAME', 'OIL_RATE', 'OIL_COST', 'OIL_GAIN',
                        'OIL_GAIN_POY_DEFAULT', 'OPU', 'WASTE_PERC')
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

-- ============================================================
-- PART 4: post-condition report
-- ============================================================
DO $$
DECLARE
    v_class    TEXT;
    v_maps     INT;
    v_moved    INT;
    v_mel      INT;
    v_missing  INT;
    v_no_opu   INT;
    v_capp     INT;
BEGIN
    SELECT COUNT(*) INTO v_capp FROM cost_product_applicable_param WHERE capp_created_by = 'migration_000572';
    SELECT cpt_oil_class INTO v_class FROM cost_product_type WHERE cpt_type_code = 'MEL';
    SELECT COUNT(*) INTO v_maps FROM cost_product_type_oil_group WHERE cptog_created_by = 'migration_000572';
    SELECT COUNT(*) INTO v_moved FROM cost_product_type_backfill_572;
    SELECT COUNT(*) INTO v_mel
    FROM cost_product_master pm
    JOIN tmp_melange_572 k ON k.legacy_id = pm.cpm_flex_02
    JOIN cost_product_type mel ON mel.cpt_type_id = pm.cpm_product_type_id AND mel.cpt_type_code = 'MEL';
    SELECT COUNT(*) INTO v_missing
    FROM tmp_melange_572 k
    WHERE NOT EXISTS (SELECT 1 FROM cost_product_master pm WHERE pm.cpm_flex_02 = k.legacy_id);
    SELECT COUNT(*) INTO v_no_opu
    FROM cost_product_master pm
    JOIN tmp_melange_572 k ON k.legacy_id = pm.cpm_flex_02
    WHERE NOT EXISTS (
        SELECT 1
        FROM cost_product_parameter cpp
        JOIN mst_parameter mp ON mp.id = cpp.cpp_param_id AND mp.param_code = 'OPU' AND mp.deleted_at IS NULL
        WHERE cpp.cpp_product_sys_id = pm.cpm_product_sys_id
          AND cpp.cpp_value_numeric IS NOT NULL
    );
    RAISE NOTICE '000572: MEL oil class = %, MEL oil-group mappings copied by 000572 = %', v_class, v_maps;
    RAISE NOTICE '000572: products moved to MEL (backup rows) = %, melange list products now MEL = % / 174, list ids not found = %',
        v_moved, v_mel, v_missing;
    RAISE NOTICE '000572: oil CAPP rows added by 000572 = %', v_capp;
    RAISE NOTICE '000572: melange products without an OPU value (OIL_COST = 0) = %', v_no_opu;
END $$;

COMMIT;
