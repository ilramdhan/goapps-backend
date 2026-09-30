-- 000546 — Loss-param dedup + V-loss formulas.
--
-- Design: docs/superpowers/specs/2026-09-30-loss-param-dedup-vloss-design.md
-- (workspace root). Decisions D1-D6 confirmed by the user 2026-09-30.
--
-- WHY THIS EXISTS
-- (1) 000470 / 000514 backfilled NS_LOSS_TYPE / BC_LOSS_TYPE (MASTER_LOOKUP ->
--     PRODUCT_GRADE) by copying the legacy STD_VALUE_LOSS / VALUE_LOSS text,
--     which holds the grade NAME ('Type 2 NS'). The Product Master combobox
--     stores and matches the lookup CODE (mst_lookup_master.lm_code_field =
--     pg_code, 'GRD-...'), so every backfilled product shows an empty grade.
-- (2) The three V-loss formulas seeded by 000408:43-45 multiply by
--     NON_STD_SPECIAL_PROD / BC_SPECIAL_PROD, which were never filled, and by
--     (1 - VALUE_LOSS/100) where VALUE_LOSS is a TEXT grade name. They have
--     always evaluated to 0 (see 000470 header note (b)).
--
-- WHAT THIS DOES
--   PART 1  Normalise NS_LOSS_TYPE / BC_LOSS_TYPE cpp_value_text to pg_code.
--           Match the current text by pg_code OR pg_name (TRIM, case-
--           insensitive; a code match wins). Where the current value is
--           missing, empty or unmatched, fall back to the legacy text
--           (STD_VALUE_LOSS -> NS_LOSS_TYPE, VALUE_LOSS -> BC_LOSS_TYPE).
--           Missing CPP rows are inserted; CAPP rows are ensured for the
--           triggers and their fill children.
--   PART 2  Refill the fill children from the grade (by pg_code):
--           NS_LOSS <- loss_pct (NS grade), STD_SP_AX <- std_selling_price,
--           STD_SP_BC <- sp_value (BC grade). Upsert; NULL source skipped.
--   PART 3  Rewrite the three V-loss formulas (D1/D3):
--             NON_STD_VALUE_LOSS   = (AE_PERC + A9_PERC + A_PERC) / 100.0 * NS_LOSS
--             BC_VAL_LOSS_CAPTIVE  = (CAPTIVE_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0
--             BC_VAL_LOSS_DELIVERY = (DELIVERY_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0
--           and replace their formula_param edges. Guarded on the exact
--           000408 text (000510 / 000532 pattern): a hand-edited formula is
--           left alone and reported.
--   PART 4  Deactivate STD_VALUE_LOSS, VALUE_LOSS, NON_STD_SPECIAL_PROD,
--           BC_SPECIAL_PROD (is_active = FALSE; data kept) — unless some other
--           active formula / fill group still depends on one, in which case
--           that param is left active and reported.
--   PART 5  Verification NOTICEs, including every unmatched legacy text.
--
-- REVERSIBILITY
-- Every CPP row this migration changes is first copied to
-- bak_loss_param_000546 (first run wins, ON CONFLICT DO NOTHING); rows it
-- inserts are stamped 'loss_param_dedup_000546' in cpp_created_by /
-- capp_created_by. The down migration restores the backed-up values (the
-- original grade names), deletes the inserted rows, restores the 000408
-- expressions / edges and reactivates the params.
--
-- IMPACT: QLTY_LOSS_*, CAP/DEL final and downstream costs change for every
-- product with grade percentages. Already-calculated periods must be
-- recalculated after deploy (costing sign-off required).
--
-- Idempotent: a second run changes nothing (all writes are IS DISTINCT FROM /
-- NOT EXISTS / ON CONFLICT guarded).
--
-- Postgres finance tables only. No Oracle.

BEGIN;

-- ============================================================
-- PRE-FLIGHT: every param this migration references must exist
-- ============================================================
-- The engine zero-fills unknown identifiers (compute.go buildInitialScope),
-- so a typo'd / missing param would silently compute 0. Abort instead.
DO $$
DECLARE
    v_missing TEXT;
BEGIN
    SELECT string_agg(c, ', ' ORDER BY c) INTO v_missing
    FROM unnest(ARRAY[
        'NS_LOSS_TYPE', 'BC_LOSS_TYPE', 'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC',
        'AE_PERC', 'A9_PERC', 'A_PERC', 'B_PERC', 'C_PERC',
        'CAPTIVE_COST_BEFORE_QLOSS', 'DELIVERY_COST_BEFORE_QLOSS',
        'NON_STD_VALUE_LOSS', 'BC_VAL_LOSS_CAPTIVE', 'BC_VAL_LOSS_DELIVERY'
    ]) AS c
    WHERE NOT EXISTS (
        SELECT 1 FROM mst_parameter p WHERE p.param_code = c AND p.deleted_at IS NULL
    );
    IF v_missing IS NOT NULL THEN
        RAISE EXCEPTION '000546 ABORT: required parameter(s) missing from mst_parameter: %', v_missing;
    END IF;
END $$;

-- ============================================================
-- Backup table (read by the down migration)
-- ============================================================
CREATE TABLE IF NOT EXISTS bak_loss_param_000546 (
    cpp_product_sys_id BIGINT       NOT NULL,
    cpp_param_id       UUID         NOT NULL,
    param_code         VARCHAR(50)  NOT NULL,
    old_value_text     TEXT,
    old_value_numeric  NUMERIC(20,6),
    old_value_flag     BOOLEAN,
    old_updated_at     TIMESTAMPTZ,
    old_updated_by     VARCHAR(100),
    backed_up_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (cpp_product_sys_id, cpp_param_id)
);

COMMENT ON TABLE bak_loss_param_000546 IS
    'Backup written by migration 000546 (loss-param dedup): CPP values it overwrote. Read by its down migration. Safe to drop once 000546 is final.';

-- ============================================================
-- PART 1: Grade triggers -> pg_code
-- ============================================================
-- One row per (product, trigger) that has either a current trigger value or a
-- legacy source value. resolved_code is the grade the product ends up on.
DROP TABLE IF EXISTS tmp_loss_type_000546;
CREATE TEMP TABLE tmp_loss_type_000546 ON COMMIT DROP AS
WITH map AS (
    SELECT tp.id AS trigger_id, tp.param_code AS trigger_code, lp.id AS legacy_id
    FROM (VALUES ('NS_LOSS_TYPE', 'STD_VALUE_LOSS'),
                 ('BC_LOSS_TYPE', 'VALUE_LOSS')) AS m(trigger_code, legacy_code)
    JOIN mst_parameter tp ON tp.param_code = m.trigger_code AND tp.deleted_at IS NULL
    LEFT JOIN mst_parameter lp ON lp.param_code = m.legacy_code AND lp.deleted_at IS NULL
),
prods AS (
    SELECT map.trigger_id, map.trigger_code, map.legacy_id, c.cpp_product_sys_id
    FROM map JOIN cost_product_parameter c ON c.cpp_param_id = map.trigger_id
    UNION
    SELECT map.trigger_id, map.trigger_code, map.legacy_id, c.cpp_product_sys_id
    FROM map JOIN cost_product_parameter c ON c.cpp_param_id = map.legacy_id
    WHERE NULLIF(TRIM(c.cpp_value_text), '') IS NOT NULL
)
SELECT p.cpp_product_sys_id,
       p.trigger_id,
       p.trigger_code,
       cur.cpp_value_id                     AS cur_value_id,
       cur.cpp_value_text                   AS cur_text,
       NULLIF(TRIM(leg.cpp_value_text), '') AS legacy_text,
       gc.pg_code                           AS cur_match,
       gl.pg_code                           AS legacy_match,
       COALESCE(gc.pg_code, gl.pg_code)     AS resolved_code
FROM prods p
LEFT JOIN cost_product_parameter cur
       ON cur.cpp_product_sys_id = p.cpp_product_sys_id AND cur.cpp_param_id = p.trigger_id
LEFT JOIN cost_product_parameter leg
       ON leg.cpp_product_sys_id = p.cpp_product_sys_id AND leg.cpp_param_id = p.legacy_id
LEFT JOIN LATERAL (
    SELECT g.pg_code FROM mst_product_grade g
    WHERE g.deleted_at IS NULL
      AND NULLIF(TRIM(cur.cpp_value_text), '') IS NOT NULL
      AND (UPPER(TRIM(g.pg_code)) = UPPER(TRIM(cur.cpp_value_text))
           OR UPPER(TRIM(g.pg_name)) = UPPER(TRIM(cur.cpp_value_text)))
    ORDER BY (UPPER(TRIM(g.pg_code)) = UPPER(TRIM(cur.cpp_value_text))) DESC, g.pg_code
    LIMIT 1
) gc ON TRUE
LEFT JOIN LATERAL (
    SELECT g.pg_code FROM mst_product_grade g
    WHERE g.deleted_at IS NULL
      AND NULLIF(TRIM(leg.cpp_value_text), '') IS NOT NULL
      AND (UPPER(TRIM(g.pg_code)) = UPPER(TRIM(leg.cpp_value_text))
           OR UPPER(TRIM(g.pg_name)) = UPPER(TRIM(leg.cpp_value_text)))
    ORDER BY (UPPER(TRIM(g.pg_code)) = UPPER(TRIM(leg.cpp_value_text))) DESC, g.pg_code
    LIMIT 1
) gl ON TRUE;

DO $$
DECLARE
    v_bak BIGINT;
    v_upd BIGINT;
    v_ins BIGINT;
    v_capp BIGINT;
BEGIN
    -- Back up every existing trigger row about to change (first run wins).
    INSERT INTO bak_loss_param_000546 (
        cpp_product_sys_id, cpp_param_id, param_code,
        old_value_text, old_value_numeric, old_value_flag, old_updated_at, old_updated_by
    )
    SELECT c.cpp_product_sys_id, c.cpp_param_id, t.trigger_code,
           c.cpp_value_text, c.cpp_value_numeric, c.cpp_value_flag, c.cpp_updated_at, c.cpp_updated_by
    FROM tmp_loss_type_000546 t
    JOIN cost_product_parameter c ON c.cpp_value_id = t.cur_value_id
    WHERE t.resolved_code IS NOT NULL
      AND c.cpp_value_text IS DISTINCT FROM t.resolved_code
    ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO NOTHING;
    GET DIAGNOSTICS v_bak = ROW_COUNT;

    -- Rewrite existing rows to the code. cpp_one_value_chk (000217:32) needs
    -- exactly one value column, so the other two are cleared.
    UPDATE cost_product_parameter c
    SET cpp_value_text    = t.resolved_code,
        cpp_value_numeric = NULL,
        cpp_value_flag    = NULL,
        cpp_updated_at    = NOW(),
        cpp_updated_by    = 'loss_param_dedup_000546'
    FROM tmp_loss_type_000546 t
    WHERE c.cpp_value_id = t.cur_value_id
      AND t.resolved_code IS NOT NULL
      AND c.cpp_value_text IS DISTINCT FROM t.resolved_code;
    GET DIAGNOSTICS v_upd = ROW_COUNT;

    -- Products with only the legacy text get a new trigger row.
    INSERT INTO cost_product_parameter (
        cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by
    )
    SELECT t.cpp_product_sys_id, t.trigger_id, t.resolved_code,
           'loss_param_dedup_000546', 'loss_param_dedup_000546'
    FROM tmp_loss_type_000546 t
    WHERE t.cur_value_id IS NULL
      AND t.resolved_code IS NOT NULL
    ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;
    GET DIAGNOSTICS v_ins = ROW_COUNT;

    -- CAPP for the triggers and their fill children. LoadCAPP / LoadCAPPText
    -- INNER JOIN CAPP, so a value without a checklist row is invisible.
    INSERT INTO cost_product_applicable_param (
        capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
    )
    SELECT DISTINCT t.cpp_product_sys_id, np.id, FALSE, NULL::INT, 'loss_param_dedup_000546'
    FROM tmp_loss_type_000546 t
    JOIN mst_parameter np
      ON np.deleted_at IS NULL
     AND (np.param_code = t.trigger_code
          OR (t.trigger_code = 'NS_LOSS_TYPE' AND np.param_code = 'NS_LOSS')
          OR (t.trigger_code = 'BC_LOSS_TYPE' AND np.param_code IN ('STD_SP_AX', 'STD_SP_BC')))
    WHERE t.resolved_code IS NOT NULL
    ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;
    GET DIAGNOSTICS v_capp = ROW_COUNT;

    RAISE NOTICE '000546 PART 1: trigger rows backed up=%, updated to pg_code=%, inserted=%, CAPP rows inserted=%',
        v_bak, v_upd, v_ins, v_capp;
END $$;

-- ============================================================
-- PART 2: Fill children from the grade (by pg_code)
-- ============================================================
DROP TABLE IF EXISTS tmp_loss_child_000546;
CREATE TEMP TABLE tmp_loss_child_000546 ON COMMIT DROP AS
SELECT DISTINCT ON (trg.cpp_product_sys_id, np.id)
       trg.cpp_product_sys_id, np.id AS child_id, np.param_code AS child_code,
       CASE map.source_col
           WHEN 'loss_pct'          THEN g.loss_pct
           WHEN 'std_selling_price' THEN g.std_selling_price
           WHEN 'sp_value'          THEN g.sp_value
       END::NUMERIC(20,6) AS val
FROM (VALUES
    ('NS_LOSS_TYPE', 'NS_LOSS',   'loss_pct'),
    ('BC_LOSS_TYPE', 'STD_SP_AX', 'std_selling_price'),
    ('BC_LOSS_TYPE', 'STD_SP_BC', 'sp_value')
) AS map(trigger_code, child_code, source_col)
JOIN mst_parameter tp ON tp.param_code = map.trigger_code AND tp.deleted_at IS NULL
JOIN mst_parameter np ON np.param_code = map.child_code   AND np.deleted_at IS NULL
JOIN cost_product_parameter trg ON trg.cpp_param_id = tp.id
JOIN mst_product_grade g ON g.pg_code = trg.cpp_value_text AND g.deleted_at IS NULL
ORDER BY trg.cpp_product_sys_id, np.id;

DELETE FROM tmp_loss_child_000546 WHERE val IS NULL;

DO $$
DECLARE
    v_bak BIGINT;
    v_upd BIGINT;
    v_ins BIGINT;
    v_capp BIGINT;
BEGIN
    INSERT INTO bak_loss_param_000546 (
        cpp_product_sys_id, cpp_param_id, param_code,
        old_value_text, old_value_numeric, old_value_flag, old_updated_at, old_updated_by
    )
    SELECT c.cpp_product_sys_id, c.cpp_param_id, t.child_code,
           c.cpp_value_text, c.cpp_value_numeric, c.cpp_value_flag, c.cpp_updated_at, c.cpp_updated_by
    FROM tmp_loss_child_000546 t
    JOIN cost_product_parameter c
      ON c.cpp_product_sys_id = t.cpp_product_sys_id AND c.cpp_param_id = t.child_id
    WHERE c.cpp_value_numeric IS DISTINCT FROM t.val
    ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO NOTHING;
    GET DIAGNOSTICS v_bak = ROW_COUNT;

    UPDATE cost_product_parameter c
    SET cpp_value_numeric = t.val,
        cpp_value_text    = NULL,
        cpp_value_flag    = NULL,
        cpp_updated_at    = NOW(),
        cpp_updated_by    = 'loss_param_dedup_000546'
    FROM tmp_loss_child_000546 t
    WHERE c.cpp_product_sys_id = t.cpp_product_sys_id
      AND c.cpp_param_id = t.child_id
      AND c.cpp_value_numeric IS DISTINCT FROM t.val;
    GET DIAGNOSTICS v_upd = ROW_COUNT;

    INSERT INTO cost_product_parameter (
        cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by
    )
    SELECT t.cpp_product_sys_id, t.child_id, t.val,
           'loss_param_dedup_000546', 'loss_param_dedup_000546'
    FROM tmp_loss_child_000546 t
    ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;
    GET DIAGNOSTICS v_ins = ROW_COUNT;

    INSERT INTO cost_product_applicable_param (
        capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
    )
    SELECT t.cpp_product_sys_id, t.child_id, FALSE, NULL::INT, 'loss_param_dedup_000546'
    FROM tmp_loss_child_000546 t
    ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;
    GET DIAGNOSTICS v_capp = ROW_COUNT;

    RAISE NOTICE '000546 PART 2: child rows backed up=%, updated=%, inserted=%, CAPP rows inserted=%',
        v_bak, v_upd, v_ins, v_capp;
END $$;

-- ============================================================
-- PART 3: Rewrite the three V-loss formulas + their edges
-- ============================================================
DO $$
DECLARE
    v_updated  INTEGER;
    v_deleted  INTEGER;
    v_inserted INTEGER;
    r          RECORD;
BEGIN
    UPDATE mst_formula f
    SET expression = CASE f.formula_code
            WHEN 'F_YARN_NON_STD_LOSS' THEN '(AE_PERC + A9_PERC + A_PERC) / 100.0 * NS_LOSS'
            WHEN 'F_YARN_BC_LOSS_CAP'  THEN '(CAPTIVE_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0'
            WHEN 'F_YARN_BC_LOSS_DEL'  THEN '(DELIVERY_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0'
        END,
        updated_at = NOW(),
        updated_by = 'loss_param_dedup_000546'
    WHERE f.deleted_at IS NULL
      AND (
          (f.formula_code = 'F_YARN_NON_STD_LOSS'
           AND f.expression = 'CAPTIVE_COST_BEFORE_QLOSS * (NON_STD_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)')
       OR (f.formula_code = 'F_YARN_BC_LOSS_CAP'
           AND f.expression = 'CAPTIVE_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)')
       OR (f.formula_code = 'F_YARN_BC_LOSS_DEL'
           AND f.expression = 'DELIVERY_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)')
      );
    GET DIAGNOSTICS v_updated = ROW_COUNT;

    -- Report any of the three NOT on the new text (hand-edited: left alone).
    FOR r IN
        SELECT formula_code, expression FROM mst_formula
        WHERE deleted_at IS NULL
          AND formula_code IN ('F_YARN_NON_STD_LOSS', 'F_YARN_BC_LOSS_CAP', 'F_YARN_BC_LOSS_DEL')
          AND expression NOT IN (
              '(AE_PERC + A9_PERC + A_PERC) / 100.0 * NS_LOSS',
              '(CAPTIVE_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0',
              '(DELIVERY_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0')
    LOOP
        RAISE WARNING '000546: % was hand-edited (expression %) — NOT rewritten, fix manually', r.formula_code, r.expression;
    END LOOP;

    -- Replace edges on the formulas this migration owns (marker), so edge and
    -- expression cannot drift apart.
    DELETE FROM formula_param fp
    USING mst_formula f, mst_parameter p
    WHERE fp.formula_id = f.id
      AND fp.param_id = p.id
      AND f.deleted_at IS NULL
      AND f.updated_by = 'loss_param_dedup_000546'
      AND (
          (f.formula_code = 'F_YARN_NON_STD_LOSS'
           AND p.param_code NOT IN ('AE_PERC', 'A9_PERC', 'A_PERC', 'NS_LOSS'))
       OR (f.formula_code = 'F_YARN_BC_LOSS_CAP'
           AND p.param_code NOT IN ('CAPTIVE_COST_BEFORE_QLOSS', 'STD_SP_BC', 'B_PERC', 'C_PERC'))
       OR (f.formula_code = 'F_YARN_BC_LOSS_DEL'
           AND p.param_code NOT IN ('DELIVERY_COST_BEFORE_QLOSS', 'STD_SP_BC', 'B_PERC', 'C_PERC'))
      );
    GET DIAGNOSTICS v_deleted = ROW_COUNT;

    INSERT INTO formula_param (formula_id, param_id, sort_order)
    SELECT f.id, p.id, e.sort_order
    FROM (VALUES
        ('F_YARN_NON_STD_LOSS', 'AE_PERC', 1),
        ('F_YARN_NON_STD_LOSS', 'A9_PERC', 2),
        ('F_YARN_NON_STD_LOSS', 'A_PERC',  3),
        ('F_YARN_NON_STD_LOSS', 'NS_LOSS', 4),
        ('F_YARN_BC_LOSS_CAP',  'CAPTIVE_COST_BEFORE_QLOSS', 1),
        ('F_YARN_BC_LOSS_CAP',  'STD_SP_BC', 2),
        ('F_YARN_BC_LOSS_CAP',  'B_PERC',    3),
        ('F_YARN_BC_LOSS_CAP',  'C_PERC',    4),
        ('F_YARN_BC_LOSS_DEL',  'DELIVERY_COST_BEFORE_QLOSS', 1),
        ('F_YARN_BC_LOSS_DEL',  'STD_SP_BC', 2),
        ('F_YARN_BC_LOSS_DEL',  'B_PERC',    3),
        ('F_YARN_BC_LOSS_DEL',  'C_PERC',    4)
    ) AS e(formula_code, param_code, sort_order)
    JOIN mst_formula f ON f.formula_code = e.formula_code
                      AND f.deleted_at IS NULL
                      AND f.updated_by = 'loss_param_dedup_000546'
    JOIN mst_parameter p ON p.param_code = e.param_code AND p.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id
    );
    GET DIAGNOSTICS v_inserted = ROW_COUNT;

    RAISE NOTICE '000546 PART 3: formulas rewritten=% (expect 3 first run, 0 re-run), edges deleted=% (expect 7), edges inserted=% (expect 10; the two *_COST_BEFORE_QLOSS edges are kept)',
        v_updated, v_deleted, v_inserted;
END $$;

-- ============================================================
-- PART 4: Deactivate the 4 legacy params (dependency-guarded)
-- ============================================================
DO $$
DECLARE
    r      RECORD;
    v_deps TEXT;
    v_off  INTEGER := 0;
BEGIN
    FOR r IN
        SELECT id, param_code FROM mst_parameter
        WHERE param_code IN ('STD_VALUE_LOSS', 'VALUE_LOSS', 'NON_STD_SPECIAL_PROD', 'BC_SPECIAL_PROD')
          AND deleted_at IS NULL
          AND is_active = TRUE
    LOOP
        SELECT string_agg(dep, ', ') INTO v_deps FROM (
            -- an active formula still reading it (edge or expression)
            SELECT 'formula ' || f.formula_code AS dep
            FROM mst_formula f
            WHERE f.deleted_at IS NULL AND f.is_active = TRUE
              AND (EXISTS (SELECT 1 FROM formula_param fp
                           WHERE fp.formula_id = f.id AND fp.param_id = r.id)
                   OR f.expression ~ ('\m' || r.param_code || '\M'))
            UNION ALL
            -- an active formula producing it
            SELECT 'result of ' || f.formula_code
            FROM mst_formula f
            WHERE f.deleted_at IS NULL AND f.is_active = TRUE AND f.result_param_id = r.id
            UNION ALL
            -- a fill-group child hanging off it
            SELECT 'fill child ' || c.param_code
            FROM mst_parameter c
            WHERE c.deleted_at IS NULL AND c.lookup_fill_group_code = r.param_code
        ) d;

        IF v_deps IS NOT NULL THEN
            RAISE WARNING '000546 PART 4: % left ACTIVE — still referenced by: %', r.param_code, v_deps;
            CONTINUE;
        END IF;

        UPDATE mst_parameter
        SET is_active = FALSE, updated_at = NOW(), updated_by = 'loss_param_dedup_000546'
        WHERE id = r.id;
        v_off := v_off + 1;
    END LOOP;

    RAISE NOTICE '000546 PART 4: legacy params deactivated=% (expect 4 first run)', v_off;
END $$;

-- ============================================================
-- PART 5: Verification (report only; unmatched text never aborts)
-- ============================================================
DO $$
DECLARE
    r RECORD;
    v_n BIGINT;
BEGIN
    FOR r IN
        SELECT trigger_code,
               COUNT(*)                                             AS in_scope,
               COUNT(*) FILTER (WHERE cur_match IS NOT NULL)        AS from_current,
               COUNT(*) FILTER (WHERE cur_match IS NULL
                                  AND legacy_match IS NOT NULL)     AS from_legacy,
               COUNT(*) FILTER (WHERE resolved_code IS NULL)        AS unresolved
        FROM tmp_loss_type_000546
        GROUP BY trigger_code ORDER BY trigger_code
    LOOP
        RAISE NOTICE '000546 post: % in-scope products=%, resolved from current text=%, from legacy text=%, UNRESOLVED=%',
            r.trigger_code, r.in_scope, r.from_current, r.from_legacy, r.unresolved;
    END LOOP;

    -- Distinct unmatched texts (current or legacy) — for manual fix.
    FOR r IN
        SELECT trigger_code, src, txt, COUNT(*) AS n FROM (
            SELECT trigger_code, 'current' AS src, TRIM(cur_text) AS txt
            FROM tmp_loss_type_000546
            WHERE NULLIF(TRIM(cur_text), '') IS NOT NULL AND cur_match IS NULL
            UNION ALL
            SELECT trigger_code, 'legacy', legacy_text
            FROM tmp_loss_type_000546
            WHERE legacy_text IS NOT NULL AND legacy_match IS NULL
        ) u
        GROUP BY trigger_code, src, txt
        ORDER BY trigger_code, src, n DESC, txt
    LOOP
        RAISE NOTICE '000546 unmatched: % % text "%" x %', r.trigger_code, r.src, r.txt, r.n;
    END LOOP;

    FOR r IN
        SELECT child_code, COUNT(*) AS n FROM tmp_loss_child_000546 GROUP BY child_code ORDER BY child_code
    LOOP
        RAISE NOTICE '000546 post: % filled from grade for % products', r.child_code, r.n;
    END LOOP;

    -- Products that compute BC V-loss but have no STD_SP_BC value: the engine
    -- zero-fills STD_SP_BC, so their BC loss is (cost - 0) * (B+C)% — review.
    SELECT COUNT(DISTINCT capp.capp_product_sys_id) INTO v_n
    FROM cost_product_applicable_param capp
    JOIN mst_parameter rp ON rp.id = capp.capp_param_id
                         AND rp.param_code IN ('BC_VAL_LOSS_CAPTIVE', 'BC_VAL_LOSS_DELIVERY')
                         AND rp.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM cost_product_parameter c
        JOIN mst_parameter sp ON sp.id = c.cpp_param_id AND sp.param_code = 'STD_SP_BC'
        WHERE c.cpp_product_sys_id = capp.capp_product_sys_id
          AND c.cpp_value_numeric IS NOT NULL
    );
    RAISE NOTICE '000546 post: products with BC_VAL_LOSS_* checklisted but no STD_SP_BC value = % (BC loss uses STD_SP_BC = 0 for them)', v_n;
END $$;

COMMIT;
