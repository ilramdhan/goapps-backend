-- 000563 — Backfill DELIVERY_BOB_RATE / DELIVERY_BOX_RATE values from the selected DELIVERY_PACK_CODE.
--
-- Why: before d12d129 / 000562 the fill-group lookup pointed at nonexistent columns, so products
-- saved earlier hold stale or empty rates although DELIVERY_PACK_CODE is set. This brings them to
-- exactly what the UI fill produces now (fillFromBoxBobbinCost):
--   DELIVERY_BOB_RATE <- mst_box_bobbin_cost.bobin_cost_val
--   DELIVERY_BOX_RATE <- mst_box_bobbin_cost.box_cost_val
-- Master row = bbc_code = cpp_value_text of DELIVERY_PACK_CODE, deleted_at IS NULL (code is unique
-- among non-deleted rows; the handler checks no is_active flag, so neither do we). The master is
-- NOT period-versioned for these columns and cost_product_parameter values are static per product.
-- A NULL master column is a deliberate no-fill in the handler (putOpt) -> skipped here too.
--
-- Rows: UPDATE only when the value is empty or IS DISTINCT FROM the master value. INSERT a value
-- row only where the product already has the child in cost_product_applicable_param (CAPP) but no
-- value row. CAPP rows are NEVER added (UI only shows attached params; attach is a separate act).
-- Products with empty/unknown pack code are skipped and counted.
--
-- NOTE: no recalculation is performed. Products with an existing cost calc snapshot must be
-- recalculated for costs to reflect the new rates.
--
-- Reversible: touched rows are first saved in bak_000563_delivery_rate_values (had_row=FALSE for
-- inserts). Marker: cpp_updated_by / cpp_created_by / cpp_filled_by = 'backfill_delivery_rate_000563'.
-- Idempotent: a second run finds no mismatches and changes nothing.
BEGIN;

CREATE TABLE IF NOT EXISTS bak_000563_delivery_rate_values (
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
COMMENT ON TABLE bak_000563_delivery_rate_values IS
    'Backup written by migration 000563; read by its down migration. Safe to drop once 000563 is final.';

-- Target value per (product, child param).
CREATE TEMP TABLE tmp_000563_target ON COMMIT DROP AS
SELECT pc.cpp_product_sys_id AS product_sys_id,
       ch.id                 AS param_id,
       ch.param_code         AS param_code,
       CASE ch.param_code WHEN 'DELIVERY_BOB_RATE' THEN bbc.bobin_cost_val
                          ELSE bbc.box_cost_val END AS new_value
FROM cost_product_parameter pc
JOIN mst_parameter pp ON pp.id = pc.cpp_param_id
                     AND pp.param_code = 'DELIVERY_PACK_CODE' AND pp.deleted_at IS NULL
JOIN mst_box_bobbin_cost bbc ON bbc.bbc_code = pc.cpp_value_text AND bbc.deleted_at IS NULL
JOIN mst_parameter ch ON ch.param_code IN ('DELIVERY_BOB_RATE','DELIVERY_BOX_RATE')
                     AND ch.deleted_at IS NULL
WHERE COALESCE(pc.cpp_value_text, '') <> '';

DELETE FROM tmp_000563_target WHERE new_value IS NULL;

-- Counts for skipped products.
DO $$
DECLARE n_nocode BIGINT; n_notfound BIGINT;
BEGIN
    SELECT COUNT(*) INTO n_nocode
      FROM cost_product_parameter pc
      JOIN mst_parameter pp ON pp.id = pc.cpp_param_id AND pp.param_code = 'DELIVERY_PACK_CODE' AND pp.deleted_at IS NULL
     WHERE COALESCE(pc.cpp_value_text, '') = '';
    SELECT COUNT(*) INTO n_notfound
      FROM cost_product_parameter pc
      JOIN mst_parameter pp ON pp.id = pc.cpp_param_id AND pp.param_code = 'DELIVERY_PACK_CODE' AND pp.deleted_at IS NULL
     WHERE COALESCE(pc.cpp_value_text, '') <> ''
       AND NOT EXISTS (SELECT 1 FROM mst_box_bobbin_cost b WHERE b.bbc_code = pc.cpp_value_text AND b.deleted_at IS NULL);
    RAISE NOTICE '000563 skipped-no-code products: %', n_nocode;
    RAISE NOTICE '000563 skipped-not-found products: %', n_notfound;
END $$;

-- PART 1: back up rows that will be rewritten.
INSERT INTO bak_000563_delivery_rate_values (
    product_sys_id, param_id, param_code, had_row,
    old_value_numeric, old_value_text, old_value_flag,
    old_filled_at, old_filled_by, old_updated_at, old_updated_by)
SELECT cpp.cpp_product_sys_id, cpp.cpp_param_id, t.param_code, TRUE,
       cpp.cpp_value_numeric, cpp.cpp_value_text, cpp.cpp_value_flag,
       cpp.cpp_filled_at, cpp.cpp_filled_by, cpp.cpp_updated_at, cpp.cpp_updated_by
FROM cost_product_parameter cpp
JOIN tmp_000563_target t ON t.product_sys_id = cpp.cpp_product_sys_id AND t.param_id = cpp.cpp_param_id
WHERE cpp.cpp_value_numeric IS DISTINCT FROM t.new_value
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

-- PART 2: update.
DO $$
DECLARE n BIGINT;
BEGIN
    UPDATE cost_product_parameter cpp
       SET cpp_value_numeric = t.new_value,
           cpp_value_text    = NULL,
           cpp_value_flag    = NULL,
           cpp_filled_at     = NOW(),
           cpp_filled_by     = 'backfill_delivery_rate_000563',
           cpp_updated_at    = NOW(),
           cpp_updated_by    = 'backfill_delivery_rate_000563'
      FROM tmp_000563_target t
     WHERE t.product_sys_id = cpp.cpp_product_sys_id AND t.param_id = cpp.cpp_param_id
       AND cpp.cpp_value_numeric IS DISTINCT FROM t.new_value;
    GET DIAGNOSTICS n = ROW_COUNT;
    RAISE NOTICE '000563 updated rows: %', n;
END $$;

-- PART 3: insert missing value rows where CAPP has the child attached.
INSERT INTO bak_000563_delivery_rate_values (product_sys_id, param_id, param_code, had_row)
SELECT t.product_sys_id, t.param_id, t.param_code, FALSE
FROM tmp_000563_target t
JOIN cost_product_applicable_param c
  ON c.capp_product_sys_id = t.product_sys_id AND c.capp_param_id = t.param_id
WHERE NOT EXISTS (SELECT 1 FROM cost_product_parameter x
                   WHERE x.cpp_product_sys_id = t.product_sys_id AND x.cpp_param_id = t.param_id)
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

DO $$
DECLARE n BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by)
    SELECT t.product_sys_id, t.param_id, t.new_value, 'backfill_delivery_rate_000563', 'backfill_delivery_rate_000563'
      FROM tmp_000563_target t
      JOIN cost_product_applicable_param c
        ON c.capp_product_sys_id = t.product_sys_id AND c.capp_param_id = t.param_id
     WHERE NOT EXISTS (SELECT 1 FROM cost_product_parameter x
                        WHERE x.cpp_product_sys_id = t.product_sys_id AND x.cpp_param_id = t.param_id)
    ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO NOTHING;
    GET DIAGNOSTICS n = ROW_COUNT;
    RAISE NOTICE '000563 inserted rows: %', n;
END $$;

COMMIT;
