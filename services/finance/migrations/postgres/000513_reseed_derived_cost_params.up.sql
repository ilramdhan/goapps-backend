-- Re-seed of 000469_seed_derived_cost_params (DML-only, lost in production).
--
-- WHY THIS EXISTS
-- The production ledger schema_migrations_finance reads 509 and is honest about
-- SCHEMA, but the seed DATA of 000468 / 000469 / 000470 never landed (those three
-- are DML-only and were most likely skipped through a `migrate force`). The
-- visible symptom: 10 cost-sheet export rows print "-" forever. This file restores
-- the 000469 share of that damage. It is written to be safe to run whether the
-- 000469 effect is fully absent, partially present, or already complete.
--
-- WHAT IT RESTORES (identical semantics to 000469)
--   CSV row 37 Duty, Inward, Waste          = RM_LANDED_COST - RM_RATE
--   CSV row 66 Forwarding Cost              = constant 0.024
--   CSV row 67 Domestic Cost (AX~AM)        = DELIVERY_COST_QLTY_LOSS + FORWARDING_COST
--   CSV row 95 Domestic cost w/ uneven pack = DOMESTIC_COST
--
-- Design decision (inherited, unchanged): these are engine params, NOT computed in
-- the export layer, so the value is auditable in the snapshot and reusable.
--
-- ⚠ STILL FLAGGED FOR RE-CONFIRMATION WITH THE COSTING TEAM — carried over from
-- 000469 and NOT resolved: FORWARDING_COST is seeded as a GLOBAL CONSTANT 0.024
-- because all 7 route stages of the template carry the same value. If the rate
-- ever varies by destination or by product, this must become a master table and
-- F_YARN_FORWARDING_COST must be re-modelled as a LOOKUP. Do not treat 0.024 as
-- confirmed.
--
-- ⚠ ALSO STILL FLAGGED: row 95 is an exact copy of row 67 in the template and the
-- "uneven packing" adjustment term is absent from the Oracle package, so
-- F_YARN_DOMESTIC_COST_UNEVEN is a pass-through until costing supplies the delta.
--
-- AUDIT MARKER
-- Everything written here carries 'reseed_derived_000513' in the relevant
-- *_created_by column, so the .down.sql can reverse exactly this run and never
-- touch a row that a real 000469 run (marker 'seed_derived_000469') produced.
--
-- VERIFICATION POLICY (learned from 000464)
-- 000464 used literal product_sys_id values that did not exist in production; its
-- WHERE EXISTS guard filtered everything away, the INSERT wrote zero rows, and its
-- DO-block let it pass because it only counted DEFINITIONS, never ROWS. Therefore:
--   * this file contains ZERO literal product_sys_id — product selection is
--     entirely derived from existing cost_product_applicable_param rows;
--   * the guard block below asserts on actual ROW counts for params AND formulas
--     AND formula_param edges AND CAPP rows;
--   * the 'USD' unit of measure is asserted as a PRECONDITION (PART 0) before any
--     INSERT runs, and the closing uom_id check is unconditional — a missing USD
--     row aborts the migration instead of quietly yielding uom_id NULL;
--   * every assert is a RELATIONAL property (e.g. "CAPP rows = 4 x the number of
--     products already carrying the upstream inputs"), never a coverage number
--     copied out of an old migration comment. Comments claiming "VERIFIED" in
--     earlier migrations were written against non-production environments and are
--     not evidence.

BEGIN;

-- ============================================================
-- PART 0: Preconditions
-- ============================================================
-- PART 1 below resolves uom_id through a LEFT JOIN on mst_uom, so a missing
-- 'USD' row would silently produce params with uom_id NULL. Fail loudly here,
-- BEFORE any INSERT, exactly as 000512 does.
DO $$
DECLARE
    v_usd INT;
BEGIN
    SELECT COUNT(*) INTO v_usd
      FROM mst_uom
     WHERE uom_code = 'USD' AND deleted_at IS NULL;

    RAISE NOTICE '000513 precondition: mst_uom USD rows = %', v_usd;

    IF v_usd <> 1 THEN
        RAISE EXCEPTION '000513: expected exactly 1 non-deleted mst_uom row with uom_code=''USD'', found %. Run 000374 first.', v_usd;
    END IF;
END $$;

-- Pre-state snapshot, so the NOTICE at the end can report what THIS run inserted
-- as opposed to what was already present.
CREATE TEMP TABLE m513_pre ON COMMIT DROP AS
SELECT
  (SELECT count(*) FROM mst_parameter
    WHERE param_code IN ('DUTY_INWARD_WASTE','FORWARDING_COST','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK')
      AND deleted_at IS NULL) AS n_param,
  (SELECT count(*) FROM mst_formula
    WHERE formula_code IN ('F_YARN_DUTY_INWARD_WASTE','F_YARN_FORWARDING_COST','F_YARN_DOMESTIC_COST','F_YARN_DOMESTIC_COST_UNEVEN')
      AND deleted_at IS NULL) AS n_formula,
  (SELECT count(*) FROM formula_param fp
     JOIN mst_formula f ON f.id = fp.formula_id AND f.deleted_at IS NULL
    WHERE f.formula_code IN ('F_YARN_DUTY_INWARD_WASTE','F_YARN_FORWARDING_COST','F_YARN_DOMESTIC_COST','F_YARN_DOMESTIC_COST_UNEVEN')) AS n_edge,
  (SELECT count(*) FROM cost_product_applicable_param capp
     JOIN mst_parameter p ON p.id = capp.capp_param_id AND p.deleted_at IS NULL
    WHERE p.param_code IN ('DUTY_INWARD_WASTE','FORWARDING_COST','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK')) AS n_capp;

-- ============================================================
-- PART 1: the 4 params (idempotent)
-- ============================================================

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, display_group, display_order,
    is_active, created_at, created_by
)
SELECT
    p.code, p.name, p.short_name, p.data_type, p.category,
    u.uom_id, p.default_val::NUMERIC, p.min_val::NUMERIC, p.max_val::NUMERIC,
    p.display_group, p.display_order, TRUE,
    NOW(), 'reseed_derived_000513'
FROM (VALUES
  ('DUTY_INWARD_WASTE','Duty, Inward, Waste','Duty Inward Waste','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Raw Material',11),
  ('FORWARDING_COST','Forwarding Cost','Forwarding Cost','NUMBER','RATE','USD','0.024',NULL,NULL,'Analysis',148),
  ('DOMESTIC_COST','Domestic Cost','Domestic Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Analysis',149),
  ('DOMESTIC_COST_UNEVEN_PACK','Domestic Cost with Uneven Packing','Domestic Cost Uneven Pack','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Analysis',150)
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val, display_group, display_order)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: the 4 formulas (idempotent) — must run AFTER part 1,
-- result_param_id is resolved from the params seeded above.
-- ============================================================

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT f.code, f.name, f.ftype, f.expr,
       (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1),
       f.descr, 1, TRUE, NOW(), 'reseed_derived_000513'
FROM (VALUES
  ('F_YARN_DUTY_INWARD_WASTE','Duty, Inward, Waste','CALCULATION','RM_LANDED_COST - RM_RATE','DUTY_INWARD_WASTE','Landing premium over the base RM rate (CSV row 37)'),
  ('F_YARN_FORWARDING_COST','Forwarding Cost','CONSTANT','0.024','FORWARDING_COST','Global forwarding rate per kg — UNCONFIRMED, see header (CSV row 66)'),
  ('F_YARN_DOMESTIC_COST','Domestic Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + FORWARDING_COST','DOMESTIC_COST','Delivery cost incl. quality loss plus forwarding (CSV row 67)'),
  ('F_YARN_DOMESTIC_COST_UNEVEN','Domestic Cost with Uneven Packing','CALCULATION','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK','Pass-through until costing supplies the uneven-packing delta (CSV row 95)')
) AS f(code, name, ftype, expr, result_code, descr)
WHERE (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1) IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = f.code AND deleted_at IS NULL
  );

-- ============================================================
-- PART 3: formula_param edges (idempotent) — must run AFTER part 2.
-- These drive topo-sort order in the engine.
-- ============================================================

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
  ('F_YARN_DUTY_INWARD_WASTE','RM_LANDED_COST',1),('F_YARN_DUTY_INWARD_WASTE','RM_RATE',2),
  -- F_YARN_FORWARDING_COST is a CONSTANT — no inputs, hence no edge.
  ('F_YARN_DOMESTIC_COST','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_DOMESTIC_COST','FORWARDING_COST',2),
  ('F_YARN_DOMESTIC_COST_UNEVEN','DOMESTIC_COST',1)
) AS fp(fcode, pcode, sort_order)
WHERE
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND NOT EXISTS (
    SELECT 1 FROM formula_param fp2
    WHERE fp2.formula_id = (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1)
      AND fp2.param_id   = (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1)
);

-- ============================================================
-- PART 4: checklist the 4 params into CAPP (idempotent)
-- ============================================================
-- LoadFormulas only returns formulas whose result param has a CAPP row for that
-- product (loadPerProductFormulas' JOIN). Without this the formulas exist but
-- never run, and the export keeps printing "-".
--
-- Scope is DERIVED, never literal: the product set is exactly the products that
-- already carry the upstream inputs (RM_LANDED_COST / DELIVERY_COST_QLTY_LOSS).
-- Bought-out / non-costed products stay untouched. There is no product_sys_id
-- literal anywhere in this file.

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT
    src.capp_product_sys_id,
    np.id,
    FALSE, NULL::INT, 'reseed_derived_000513'
FROM cost_product_applicable_param src
JOIN mst_parameter sp ON sp.id = src.capp_param_id AND sp.deleted_at IS NULL
CROSS JOIN mst_parameter np
WHERE sp.param_code IN ('RM_LANDED_COST', 'DELIVERY_COST_QLTY_LOSS')
  AND np.param_code IN ('DUTY_INWARD_WASTE', 'FORWARDING_COST', 'DOMESTIC_COST', 'DOMESTIC_COST_UNEVEN_PACK')
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = src.capp_product_sys_id
        AND capp.capp_param_id       = np.id
  );

-- ============================================================
-- PART 5: HARD VERIFICATION — asserts on ROWS, not on definitions
-- ============================================================
-- Every threshold below is either (a) the size of the VALUES list this very file
-- declares, or (b) a quantity re-derived from the database at runtime. No number
-- is copied from an old migration's comment.

DO $verify$
DECLARE
    pre                RECORD;
    n_param            INTEGER;
    n_formula          INTEGER;
    n_formula_bad_res  INTEGER;
    n_edge             INTEGER;
    n_edge_missing     INTEGER;
    n_base_products    BIGINT;
    n_capp             BIGINT;
    n_capp_expected    BIGINT;
    n_uom_usd          INTEGER;
    n_param_no_uom     INTEGER;
    n_real_products    BIGINT;
BEGIN
    SELECT * INTO pre FROM m513_pre;

    -- ---- params ----
    SELECT count(*) INTO n_param
      FROM mst_parameter
     WHERE param_code IN ('DUTY_INWARD_WASTE','FORWARDING_COST','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK')
       AND deleted_at IS NULL;

    -- ---- formulas (and that each one actually resolved a result param) ----
    SELECT count(*) INTO n_formula
      FROM mst_formula
     WHERE formula_code IN ('F_YARN_DUTY_INWARD_WASTE','F_YARN_FORWARDING_COST','F_YARN_DOMESTIC_COST','F_YARN_DOMESTIC_COST_UNEVEN')
       AND deleted_at IS NULL;

    SELECT count(*) INTO n_formula_bad_res
      FROM mst_formula f
      LEFT JOIN mst_parameter p ON p.id = f.result_param_id AND p.deleted_at IS NULL
     WHERE f.formula_code IN ('F_YARN_DUTY_INWARD_WASTE','F_YARN_FORWARDING_COST','F_YARN_DOMESTIC_COST','F_YARN_DOMESTIC_COST_UNEVEN')
       AND f.deleted_at IS NULL
       AND p.id IS NULL;

    -- ---- formula_param edges: assert every declared edge is present ----
    -- The expected set is re-stated from this file's own PART 3 VALUES list, so
    -- it can never drift into a "number someone measured on dev".
    SELECT count(*) INTO n_edge_missing
      FROM (VALUES
        ('F_YARN_DUTY_INWARD_WASTE','RM_LANDED_COST'),('F_YARN_DUTY_INWARD_WASTE','RM_RATE'),
        ('F_YARN_DOMESTIC_COST','DELIVERY_COST_QLTY_LOSS'),('F_YARN_DOMESTIC_COST','FORWARDING_COST'),
        ('F_YARN_DOMESTIC_COST_UNEVEN','DOMESTIC_COST')
      ) AS want(fcode, pcode)
     WHERE NOT EXISTS (
        SELECT 1
          FROM formula_param fp
          JOIN mst_formula   f ON f.id = fp.formula_id AND f.deleted_at IS NULL
          JOIN mst_parameter p ON p.id = fp.param_id   AND p.deleted_at IS NULL
         WHERE f.formula_code = want.fcode
           AND p.param_code   = want.pcode
     );

    SELECT count(*) INTO n_edge
      FROM formula_param fp
      JOIN mst_formula f ON f.id = fp.formula_id AND f.deleted_at IS NULL
     WHERE f.formula_code IN ('F_YARN_DUTY_INWARD_WASTE','F_YARN_FORWARDING_COST','F_YARN_DOMESTIC_COST','F_YARN_DOMESTIC_COST_UNEVEN');

    -- ---- CAPP: relational expectation, re-derived at runtime ----
    SELECT count(DISTINCT src.capp_product_sys_id) INTO n_base_products
      FROM cost_product_applicable_param src
      JOIN mst_parameter sp ON sp.id = src.capp_param_id AND sp.deleted_at IS NULL
     WHERE sp.param_code IN ('RM_LANDED_COST','DELIVERY_COST_QLTY_LOSS');

    SELECT count(*) INTO n_capp
      FROM cost_product_applicable_param capp
      JOIN mst_parameter p ON p.id = capp.capp_param_id AND p.deleted_at IS NULL
     WHERE p.param_code IN ('DUTY_INWARD_WASTE','FORWARDING_COST','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK');

    n_capp_expected := n_base_products * 4;

    -- ---- UOM resolution (PART 1 uses a LEFT JOIN, so a missing USD row would
    -- ---- silently leave uom_id NULL instead of failing) ----
    SELECT count(*) INTO n_uom_usd FROM mst_uom WHERE uom_code = 'USD' AND deleted_at IS NULL;
    SELECT count(*) INTO n_param_no_uom
      FROM mst_parameter
     WHERE param_code IN ('DUTY_INWARD_WASTE','FORWARDING_COST','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK')
       AND deleted_at IS NULL AND uom_id IS NULL;

    RAISE NOTICE '000513: params %->% (this run inserted %), formulas %->% (inserted %), formula_param edges %->% (inserted %), CAPP rows %->% (inserted %)',
        pre.n_param, n_param, n_param - pre.n_param,
        pre.n_formula, n_formula, n_formula - pre.n_formula,
        pre.n_edge, n_edge, n_edge - pre.n_edge,
        pre.n_capp, n_capp, n_capp - pre.n_capp;
    RAISE NOTICE '000513: upstream product base (products carrying RM_LANDED_COST or DELIVERY_COST_QLTY_LOSS) = %, expected CAPP rows = %',
        n_base_products, n_capp_expected;

    IF n_param <> 4 THEN
        RAISE EXCEPTION '000513: expected 4 derived params present, found % — PART 1 wrote nothing usable', n_param;
    END IF;

    -- PART 0 already proved exactly one non-deleted USD row exists, so this is
    -- no longer conditional on n_uom_usd: any NULL uom_id here is a real defect.
    IF n_param_no_uom > 0 THEN
        RAISE EXCEPTION '000513: mst_uom has USD (% row(s)) but % of the 4 params ended up with NULL uom_id', n_uom_usd, n_param_no_uom;
    END IF;

    IF n_formula <> 4 THEN
        RAISE EXCEPTION '000513: expected 4 formulas present, found % — PART 2 wrote nothing usable', n_formula;
    END IF;

    IF n_formula_bad_res > 0 THEN
        RAISE EXCEPTION '000513: % formula(s) have a result_param_id that resolves to no live param', n_formula_bad_res;
    END IF;

    IF n_edge_missing > 0 THEN
        RAISE EXCEPTION '000513: % declared formula_param edge(s) missing — PART 3 silently filtered rows away (upstream param codes RM_LANDED_COST / RM_RATE / DELIVERY_COST_QLTY_LOSS may not exist)', n_edge_missing;
    END IF;

    -- ---- Is this a real (populated) catalogue, or a migration-only database? ----
    -- The only products any migration ever inserts are the TXFX_% fixtures of
    -- 000236 / 000239. A database built by running migrations alone therefore
    -- contains fixtures and nothing else, and those fixtures never carry
    -- RM_LANDED_COST / DELIVERY_COST_QLTY_LOSS: 000242 cross-joined products
    -- against the formula inputs that existed AT THAT TIME, and both of those
    -- param codes were only created later (000381 / 000407). No migration
    -- re-runs that cross join afterwards, so n_base_products = 0 is the CORRECT
    -- and unavoidable outcome on a fresh database.
    --
    -- Real products arrive only via the application / Oracle import, never via
    -- migrations. So "at least one non-fixture product exists" is what separates
    -- a populated database from a migration-only one.
    SELECT COUNT(*) INTO n_real_products
      FROM cost_product_master
     WHERE cpm_product_code NOT LIKE 'TXFX\_%';

    -- Why zero is fatal in one case and fine in the other: on a populated
    -- database (production carries ~13k products) an empty derived base is the
    -- exact 000464 failure mode — every INSERT writes zero rows while each
    -- individual statement still "succeeds", and the export silently keeps
    -- printing "-". On a migration-only database (CI) there is simply nothing
    -- to derive from, and failing there would block the whole migration chain
    -- over an absence of data that is expected by construction.
    --
    -- Note this guard is a BUSINESS-VOLUME assertion, not a correctness one.
    -- Correctness is still enforced unconditionally by the n_capp shortfall
    -- check below: when the base is 0 the expectation is 0, and 0 >= 0 passes
    -- for the right reason rather than by being skipped.
    IF n_real_products > 0 AND n_base_products = 0 THEN
        RAISE EXCEPTION '000513: no product carries RM_LANDED_COST or DELIVERY_COST_QLTY_LOSS in cost_product_applicable_param — CAPP backfill would write 0 rows and the export would keep printing "-"';
    ELSIF n_base_products = 0 THEN
        RAISE NOTICE '000513: no product carries RM_LANDED_COST or DELIVERY_COST_QLTY_LOSS (non-fixture products = %) — CAPP backfill wrote 0 rows. This is EXPECTED on a migration-only database such as CI, where only the TXFX_%% fixtures exist. On production it would mean the derived-cost columns silently export as "-".', n_real_products;
    END IF;

    IF n_capp < n_capp_expected THEN
        RAISE EXCEPTION '000513: CAPP rows for the 4 derived params = %, expected % (= 4 x % base products)',
            n_capp, n_capp_expected, n_base_products;
    END IF;
END $verify$;

COMMIT;
