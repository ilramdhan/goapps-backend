-- 000564 — Box/bobbin pack rates depend on the calculation type (Marketing requirement).
--
--   ACTUAL (valuation)        -> VAL rates (mst_box_bobbin_cost.bobin_cost_val / box_cost_val)
--   FORECAST and SELLING (mkt)-> MKT rates (mst_box_bobbin_cost.bobin_cost      / box_cost)
-- for BOTH captive (F_YARN_CAP_PACK, row 42) and delivery (F_YARN_DEL_PACK, row 43).
--
-- Engine: IS_ACTUAL (1 for ACTUAL else 0) is injected by costcalc (injectCalcTypeFlags), like IS_POY;
-- it is NOT an mst_parameter row and needs no formula_param edge.
--
-- Existing params keep their source (numbers stay stable):
--   CAPTIVE_BOB_RATE/BOX_RATE  (CAPTIVE_PACK_CODE) = MKT side (source bbcr_*_mkt: latest-period rate history)
--   DELIVERY_BOB_RATE/BOX_RATE (DELIVERY_PACK_CODE)= VAL side (source bobin_cost_val / box_cost_val, 000562)
-- New MASTER_LOOKUP children:
--   CAP_BOB_RATE_VAL / CAP_BOX_RATE_VAL   (CAPTIVE_PACK_CODE,  bobin_cost_val / box_cost_val)
--   DEL_BOB_RATE_MKT / DEL_BOX_RATE_MKT   (DELIVERY_PACK_CODE, bobin_cost / box_cost)
-- NOTE the asymmetry: captive MKT reads rate history (bbcr_*_mkt) whereas delivery MKT reads the
-- master columns. Switch a source column on the web (Master Parameter -> Source Column) if desired.
--
-- Formulas are rewritten only when the live expression equals a known text AND the result param
-- matches (hand edits are never clobbered). The original text/description are saved in
-- bak_000564_formula for an exact down. New params are attached (CAPP) to every product that has
-- the parent pack code attached; values are backfilled from the master row (bbc_code = pack code),
-- NULL master values are skipped (same semantics as 000563). Marker: 'pack_rates_000564'.
-- No recalculation is performed: recalc is needed for new numbers to appear.
BEGIN;

-- PART 1: params
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, lookup_fill_group_code, lookup_source_column,
    is_active, created_at, created_by
)
SELECT v.code, v.name, v.name, 'NUMBER', 'MASTER_LOOKUP',
       (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
       'Finance', FALSE, FALSE, 'Packing', v.ord, v.note, v.grp, v.src,
       TRUE, NOW(), 'seed_000564'
FROM (VALUES
  ('CAP_BOB_RATE_VAL','Captive Bobbin Rate VAL', 44,'Captive bobbin rate (valuation) used for ACTUAL.','CAPTIVE_PACK_CODE','bobin_cost_val'),
  ('CAP_BOX_RATE_VAL','Captive Box Rate VAL',    44,'Captive box rate (valuation) used for ACTUAL.','CAPTIVE_PACK_CODE','box_cost_val'),
  ('DEL_BOB_RATE_MKT','Delivery Bobbin Rate MKT',50,'Delivery bobbin rate (marketing) used for FORECAST/SELLING.','DELIVERY_PACK_CODE','bobin_cost'),
  ('DEL_BOX_RATE_MKT','Delivery Box Rate MKT',   50,'Delivery box rate (marketing) used for FORECAST/SELLING.','DELIVERY_PACK_CODE','box_cost')
) AS v(code, name, ord, note, grp, src)
WHERE NOT EXISTS (SELECT 1 FROM mst_parameter x WHERE x.param_code = v.code AND x.deleted_at IS NULL);

-- PART 2: back up + rewrite formulas
CREATE TABLE IF NOT EXISTS bak_000564_formula (
    formula_code    VARCHAR(100) PRIMARY KEY,
    old_expression  TEXT NOT NULL,
    old_description TEXT,
    old_updated_at  TIMESTAMPTZ,
    old_updated_by  VARCHAR(100),
    backed_up_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE bak_000564_formula IS 'Backup written by migration 000564; read by its down migration.';

INSERT INTO bak_000564_formula (formula_code, old_expression, old_description, old_updated_at, old_updated_by)
SELECT f.formula_code, f.expression, f.description, f.updated_at, f.updated_by
FROM mst_formula f
JOIN mst_parameter p ON p.id = f.result_param_id AND p.deleted_at IS NULL
WHERE f.deleted_at IS NULL
  AND ((f.formula_code = 'F_YARN_CAP_PACK' AND p.param_code = 'CAPTIVE_PACK_COST'
        AND f.expression = 'IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0)')
    OR (f.formula_code = 'F_YARN_DEL_PACK' AND p.param_code = 'DELIVERY_PACK_COST'
        AND f.expression IN (
            'DELIVERY_BOX_WT > 0 ? ((DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE) + DELIVERY_BOX_RATE) / DELIVERY_BOX_WT : 0',
            'DELIVERY_BOX_WT > 0 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE) / DELIVERY_BOX_WT : 0')))
ON CONFLICT (formula_code) DO NOTHING;

UPDATE mst_formula f
SET expression  = 'IS_POY == 1 ? CAP_PACK_POY_DEFAULT : (CAPTIVE_BOX_WT > 0 ? (IS_ACTUAL == 1 ? (CAPTIVE_NO_OF_BOB * CAP_BOB_RATE_VAL + CAP_BOX_RATE_VAL) : (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE)) / CAPTIVE_BOX_WT : 0)',
    description = 'Captive pack cost per kg. POY: CAP_PACK_POY_DEFAULT; others: (bobbins*bob rate + box rate)/box weight, VAL rates for ACTUAL, MKT rates otherwise.',
    updated_at  = NOW(),
    updated_by  = 'pack_rates_000564'
WHERE f.formula_code = 'F_YARN_CAP_PACK' AND f.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM bak_000564_formula b WHERE b.formula_code = f.formula_code AND b.old_expression = f.expression)
  AND EXISTS (SELECT 1 FROM mst_parameter p WHERE p.id = f.result_param_id AND p.param_code = 'CAPTIVE_PACK_COST' AND p.deleted_at IS NULL)
  AND (SELECT COUNT(*) FROM mst_parameter WHERE param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL') AND deleted_at IS NULL) = 2;

UPDATE mst_formula f
SET expression  = 'DELIVERY_BOX_WT > 0 ? (IS_ACTUAL == 1 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE) : (DELIVERY_NO_OF_BOB * DEL_BOB_RATE_MKT + DEL_BOX_RATE_MKT)) / DELIVERY_BOX_WT : 0',
    description = 'Delivery pack cost per kg. VAL rates for ACTUAL, MKT rates (DEL_*_MKT) for FORECAST/SELLING.',
    updated_at  = NOW(),
    updated_by  = 'pack_rates_000564'
WHERE f.formula_code = 'F_YARN_DEL_PACK' AND f.deleted_at IS NULL
  AND EXISTS (SELECT 1 FROM bak_000564_formula b WHERE b.formula_code = f.formula_code AND b.old_expression = f.expression)
  AND EXISTS (SELECT 1 FROM mst_parameter p WHERE p.id = f.result_param_id AND p.param_code = 'DELIVERY_PACK_COST' AND p.deleted_at IS NULL)
  AND (SELECT COUNT(*) FROM mst_parameter WHERE param_code IN ('DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT') AND deleted_at IS NULL) = 2;

-- PART 3: edges (only for formulas this migration rewrote)
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id,
       COALESCE((SELECT MAX(fp2.sort_order) FROM formula_param fp2 WHERE fp2.formula_id = f.id), 0)
         + ROW_NUMBER() OVER (PARTITION BY f.id ORDER BY p.param_code)
FROM mst_formula f
JOIN mst_parameter p ON p.deleted_at IS NULL
 AND ((f.formula_code = 'F_YARN_CAP_PACK' AND p.param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL'))
   OR (f.formula_code = 'F_YARN_DEL_PACK' AND p.param_code IN ('DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')))
WHERE f.deleted_at IS NULL AND f.updated_by = 'pack_rates_000564'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- PART 4: attach children to every product that has the parent attached
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by)
SELECT parent.capp_product_sys_id, ch.id, FALSE, NULL::INT, 'pack_rates_000564'
FROM cost_product_applicable_param parent
JOIN mst_parameter pp ON pp.id = parent.capp_param_id AND pp.deleted_at IS NULL
JOIN mst_parameter ch ON ch.deleted_at IS NULL AND ch.lookup_fill_group_code = pp.param_code
 AND ch.param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')
WHERE pp.param_code IN ('CAPTIVE_PACK_CODE','DELIVERY_PACK_CODE')
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

-- PART 5: backfill values from the master row
CREATE TABLE IF NOT EXISTS bak_000564_pack_rate_values (
    product_sys_id    BIGINT       NOT NULL,
    param_id          UUID         NOT NULL,
    param_code        VARCHAR(100) NOT NULL,
    had_row           BOOLEAN      NOT NULL,
    old_value_numeric NUMERIC(20,6),
    old_value_text    TEXT,
    old_value_flag    BOOLEAN,
    old_filled_at     TIMESTAMPTZ,
    old_filled_by     VARCHAR(100),
    old_updated_at    TIMESTAMPTZ,
    old_updated_by    VARCHAR(100),
    backed_up_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (product_sys_id, param_id)
);
COMMENT ON TABLE bak_000564_pack_rate_values IS 'Backup written by migration 000564; read by its down migration.';

CREATE TEMP TABLE tmp_000564_target ON COMMIT DROP AS
SELECT pc.cpp_product_sys_id AS product_sys_id, ch.id AS param_id, ch.param_code,
       CASE ch.param_code WHEN 'CAP_BOB_RATE_VAL' THEN bbc.bobin_cost_val
                          WHEN 'CAP_BOX_RATE_VAL' THEN bbc.box_cost_val
                          WHEN 'DEL_BOB_RATE_MKT' THEN bbc.bobin_cost
                          ELSE bbc.box_cost END AS new_value
FROM cost_product_parameter pc
JOIN mst_parameter pp ON pp.id = pc.cpp_param_id AND pp.deleted_at IS NULL
 AND pp.param_code IN ('CAPTIVE_PACK_CODE','DELIVERY_PACK_CODE')
JOIN mst_box_bobbin_cost bbc ON bbc.bbc_code = pc.cpp_value_text AND bbc.deleted_at IS NULL
JOIN mst_parameter ch ON ch.deleted_at IS NULL AND ch.lookup_fill_group_code = pp.param_code
 AND ch.param_code IN ('CAP_BOB_RATE_VAL','CAP_BOX_RATE_VAL','DEL_BOB_RATE_MKT','DEL_BOX_RATE_MKT')
WHERE COALESCE(pc.cpp_value_text, '') <> '';

DELETE FROM tmp_000564_target WHERE new_value IS NULL;

DO $$
DECLARE n_nocode BIGINT; n_notfound BIGINT;
BEGIN
    SELECT COUNT(*) INTO n_nocode
      FROM cost_product_parameter pc
      JOIN mst_parameter pp ON pp.id = pc.cpp_param_id AND pp.deleted_at IS NULL
       AND pp.param_code IN ('CAPTIVE_PACK_CODE','DELIVERY_PACK_CODE')
     WHERE COALESCE(pc.cpp_value_text, '') = '';
    SELECT COUNT(*) INTO n_notfound
      FROM cost_product_parameter pc
      JOIN mst_parameter pp ON pp.id = pc.cpp_param_id AND pp.deleted_at IS NULL
       AND pp.param_code IN ('CAPTIVE_PACK_CODE','DELIVERY_PACK_CODE')
     WHERE COALESCE(pc.cpp_value_text, '') <> ''
       AND NOT EXISTS (SELECT 1 FROM mst_box_bobbin_cost b WHERE b.bbc_code = pc.cpp_value_text AND b.deleted_at IS NULL);
    RAISE NOTICE '000564 skipped-no-code pack-code rows: %', n_nocode;
    RAISE NOTICE '000564 skipped-not-found pack-code rows: %', n_notfound;
END $$;

-- back up existing rows to be rewritten (normally none), then update
INSERT INTO bak_000564_pack_rate_values (
    product_sys_id, param_id, param_code, had_row,
    old_value_numeric, old_value_text, old_value_flag, old_filled_at, old_filled_by, old_updated_at, old_updated_by)
SELECT cpp.cpp_product_sys_id, cpp.cpp_param_id, t.param_code, TRUE,
       cpp.cpp_value_numeric, cpp.cpp_value_text, cpp.cpp_value_flag,
       cpp.cpp_filled_at, cpp.cpp_filled_by, cpp.cpp_updated_at, cpp.cpp_updated_by
FROM cost_product_parameter cpp
JOIN tmp_000564_target t ON t.product_sys_id = cpp.cpp_product_sys_id AND t.param_id = cpp.cpp_param_id
WHERE cpp.cpp_value_numeric IS DISTINCT FROM t.new_value
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

DO $$
DECLARE n BIGINT;
BEGIN
    UPDATE cost_product_parameter cpp
       SET cpp_value_numeric = t.new_value, cpp_value_text = NULL, cpp_value_flag = NULL,
           cpp_filled_at = NOW(), cpp_filled_by = 'pack_rates_000564',
           cpp_updated_at = NOW(), cpp_updated_by = 'pack_rates_000564'
      FROM tmp_000564_target t
     WHERE t.product_sys_id = cpp.cpp_product_sys_id AND t.param_id = cpp.cpp_param_id
       AND cpp.cpp_value_numeric IS DISTINCT FROM t.new_value;
    GET DIAGNOSTICS n = ROW_COUNT;
    RAISE NOTICE '000564 updated value rows: %', n;
END $$;

-- insert missing value rows only where CAPP has the child (PART 4 guarantees it for matched products)
INSERT INTO bak_000564_pack_rate_values (product_sys_id, param_id, param_code, had_row)
SELECT t.product_sys_id, t.param_id, t.param_code, FALSE
FROM tmp_000564_target t
JOIN cost_product_applicable_param c ON c.capp_product_sys_id = t.product_sys_id AND c.capp_param_id = t.param_id
WHERE NOT EXISTS (SELECT 1 FROM cost_product_parameter x
                   WHERE x.cpp_product_sys_id = t.product_sys_id AND x.cpp_param_id = t.param_id)
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

DO $$
DECLARE n BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
    SELECT t.product_sys_id, t.param_id, t.new_value, 'pack_rates_000564', 'pack_rates_000564'
      FROM tmp_000564_target t
      JOIN cost_product_applicable_param c ON c.capp_product_sys_id = t.product_sys_id AND c.capp_param_id = t.param_id
     WHERE NOT EXISTS (SELECT 1 FROM cost_product_parameter x
                        WHERE x.cpp_product_sys_id = t.product_sys_id AND x.cpp_param_id = t.param_id)
    ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO NOTHING;
    GET DIAGNOSTICS n = ROW_COUNT;
    RAISE NOTICE '000564 inserted value rows: %', n;
END $$;

DO $$
DECLARE v_cap TEXT; v_del TEXT;
BEGIN
    SELECT updated_by INTO v_cap FROM mst_formula WHERE formula_code='F_YARN_CAP_PACK' AND deleted_at IS NULL;
    SELECT updated_by INTO v_del FROM mst_formula WHERE formula_code='F_YARN_DEL_PACK' AND deleted_at IS NULL;
    IF v_cap IS DISTINCT FROM 'pack_rates_000564' THEN
        RAISE NOTICE '000564: F_YARN_CAP_PACK NOT rewritten (missing or expression drifted) — fix by hand.';
    END IF;
    IF v_del IS DISTINCT FROM 'pack_rates_000564' THEN
        RAISE NOTICE '000564: F_YARN_DEL_PACK NOT rewritten (missing or expression drifted) — fix by hand.';
    END IF;
END $$;

COMMIT;
