-- 000515: re-seed COST_STAGE_OUT as the EXPLICIT terminal sink of the cost engine.
--
-- WHY THIS EXISTS
-- compute.go resolves the final cost of a product in three steps (compute.go:205-213):
--   a) scope["COST_STAGE_OUT"]  -- explicit terminal sink, ScopeKeyFinalCost (compute.go:41)
--   b) resolveFinalCost -> findTerminalFormula  -- HEURISTIC: deepest formula, ties
--      broken by FormulaCode ASC
--   c) COST_RM_TOTAL
-- Branch (a) is currently DEAD: 000406_clear_old_param_formula_seeds.up.sql lines 5-9
-- delete EVERY row of formula_param / mst_formula / cost_product_parameter /
-- cost_product_applicable_param / mst_parameter with no filter at all, so the param
-- COST_STAGE_OUT (originally 000381:202) and the formula F_YARN_STAGE_OUT
-- (000390:73, 000391:44) are gone, and 000407 never re-seeded COST_STAGE_OUT.
-- Consequence: every product falls through to branch (b) and cpc_cost_per_unit is
-- decided by a depth + alphabetical tie-break, i.e. by accident rather than by design.
-- This migration restores branch (a) so the answer comes from the MODEL.
--
-- WHAT IT SEEDS
--   param   COST_STAGE_OUT   'Final Stage Cost (engine sink)'  (CALCULATED, USD)
--   formula F_YARN_STAGE_OUT CALCULATION, expression = 'DOMESTIC_COST'
--   edge    (F_YARN_STAGE_OUT, DOMESTIC_COST, sort_order 1)
--   CAPP    one COST_STAGE_OUT row per NON-MB product that already carries a
--           DOMESTIC_COST CAPP row
-- DOMESTIC_COST itself (plus its formula F_YARN_DOMESTIC_COST at 000513:133 and its
-- CAPP rows at 000513:178-196) comes from 000513 PART 1.
--
-- WHY DOMESTIC_COST IS THE PASS-THROUGH SOURCE (user decision 2026-09-17, not inferred here)
-- Option A (DOMESTIC_COST) was chosen over Option B (VOLUME_BUCKET_4_DEL_COST) because the
-- sink must not be tied to a bucket number that has no documented business basis.
--
-- CORRECTION 2026-09-17 (V-22 supersedes V-21): an earlier draft of this header claimed
-- "V-21 showed legacy has NO intermediate VB LOSS step at all". That claim is WRONG and is
-- retracted. V22-O1.A proved TOP 113-117 (VOLUME_BUCKET_1..5_LOSS) DO exist, are referenced
-- as operands by TOP 118-122, and hold 109,370 stored value rows. What is absent is only
-- their FORMULA rows in cst_yarn_formula_calc -- the values are stored data, not computed
-- there. The costing team has since confirmed that some params are calculated on a separate
-- PHP side rather than in Oracle, which explains the missing formula rows.
-- The Option A decision itself is UNCHANGED: its primary reason (no business basis for
-- anchoring the sink to a bucket number) stands on its own. Only this supporting argument
-- is withdrawn.
-- Legacy Oracle treats TOP 121 = "Final Ex-Factory Cost" = V4 = Dom Cost as the
-- official cost per unit: PKG_YARN_MARKETING.pkb:1030-1056 and :1171-1177 both read
-- the same CYCC_TOP_121_DATA_VALUE column.
--
-- WHY MB IS EXCLUDED
-- LoadUpstreamCosts (loader.go:936-939) reads
--     CASE WHEN pt.cpt_type_code = 'MB' THEN pc.cpc_cost_per_unit
--          ELSE COALESCE(pc.cpc_captive_cost, 0) END
-- so for NON-MB products the downstream RM chain consumes cpc_captive_cost and
-- changing cpc_cost_per_unit is inert for upstream/downstream propagation. For MB it
-- is the opposite: cpc_cost_per_unit IS the propagated value, and MB additionally has
-- its own calculation path (MB batch trigger pushing into the head). Giving MB a
-- COST_STAGE_OUT sink could therefore change MB's propagated cost and break that
-- path. MB is identified by cost_product_type.cpt_type_code = 'MB'
-- (000100_create_cost_product_type.up.sql:7, seeded by 000450:1-3), joined through
-- cost_product_master.cpm_product_type_id (000106_create_cost_product_master.up.sql:9-10).
--
-- HONEST IMPACT STATEMENT
-- After this migration + a recalculation, cpc_cost_per_unit for non-MB products is
-- exactly DOMESTIC_COST. In practice the STORED NUMBERS WILL BARELY MOVE: the value
-- the heuristic currently picks is DOMESTIC_COST_UNEVEN_PACK, which 000513:134 defines
-- as a pure pass-through of DOMESTIC_COST. This change fixes the MECHANISM (the
-- terminal cost becomes a designed, auditable choice instead of a depth + FormulaCode
-- ASC accident), not primarily the figures.
--
-- ⚠ FLAG — NOT CONFIRMED: whether the official cost/unit should INCLUDE forwarding.
-- Legacy is ambiguous: fgetFinalExFactoryCost does NOT add forwarding, but fget_V4 is
-- invoked as "+ getPrdFowarding" in SQL. DOMESTIC_COST = DELIVERY_COST_QLTY_LOSS +
-- FORWARDING_COST (000513:133), so it INCLUDES forwarding. If costing later rules that
-- forwarding is excluded, the pass-through source of F_YARN_STAGE_OUT must become
-- DELIVERY_COST_QLTY_LOSS instead — a difference of 0.024/kg.
--
-- ⚠ FLAG — inherited from 000513:20-25: FORWARDING_COST = 0.024 is still a seeded
-- global constant that the costing team has never confirmed.
--
-- AUDIT MARKER
-- Every row written here carries 'seed_stage_out_000515' in created_by /
-- capp_created_by, so the .down.sql reverses exactly this run and nothing else.
--
-- VERIFICATION POLICY (learned from 000464 and from commit e34ca01)
--   * ZERO literal product_sys_id anywhere; product scope is derived RELATIONALLY
--     from existing CAPP rows, so a staging/production id-namespace mismatch cannot
--     silently filter everything away;
--   * PART 5 asserts on ROW counts (params, formulas, edges, CAPP), never on the
--     number of DEFINITIONS this file declares -- that was the silent 000464 failure;
--   * the CAPP expectation is re-derived at runtime, never hardcoded;
--   * the volume guard is CONDITIONAL on the catalogue actually being populated,
--     because CI builds the database from migrations alone and only ever contains the
--     TXFX_% fixtures. An unconditional guard there breaks the whole chain (the bug
--     fixed in e34ca01).

BEGIN;

-- ============================================================
-- PART 0: Preconditions
-- ============================================================
-- PART 1 resolves uom_id via LEFT JOIN on mst_uom, so a missing 'USD' row would
-- silently yield uom_id NULL. Fail loudly BEFORE any INSERT, exactly as 000513 does.
-- Also assert the upstream param DOMESTIC_COST exists: without it PART 2/3 would
-- filter themselves away and leave a formula with no input edge.
DO $$
DECLARE
    v_usd      INT;
    v_domestic INT;
BEGIN
    SELECT COUNT(*) INTO v_usd
      FROM mst_uom
     WHERE uom_code = 'USD' AND deleted_at IS NULL;

    RAISE NOTICE '000515 precondition: mst_uom USD rows = %', v_usd;

    IF v_usd <> 1 THEN
        RAISE EXCEPTION '000515: expected exactly 1 non-deleted mst_uom row with uom_code=''USD'', found %. Run 000374 first.', v_usd;
    END IF;

    SELECT COUNT(*) INTO v_domestic
      FROM mst_parameter
     WHERE param_code = 'DOMESTIC_COST' AND deleted_at IS NULL;

    RAISE NOTICE '000515 precondition: mst_parameter DOMESTIC_COST rows = %', v_domestic;

    IF v_domestic <> 1 THEN
        RAISE EXCEPTION '000515: expected exactly 1 non-deleted mst_parameter row with param_code=''DOMESTIC_COST'', found %. Run 000513 first — F_YARN_STAGE_OUT passes DOMESTIC_COST through.', v_domestic;
    END IF;
END $$;

-- Pre-state snapshot so the closing NOTICE can report what THIS run inserted as
-- opposed to what was already present.
CREATE TEMP TABLE m515_pre ON COMMIT DROP AS
SELECT
  (SELECT count(*) FROM mst_parameter
    WHERE param_code = 'COST_STAGE_OUT' AND deleted_at IS NULL) AS n_param,
  (SELECT count(*) FROM mst_formula
    WHERE formula_code = 'F_YARN_STAGE_OUT' AND deleted_at IS NULL) AS n_formula,
  (SELECT count(*) FROM formula_param fp
     JOIN mst_formula f ON f.id = fp.formula_id AND f.deleted_at IS NULL
    WHERE f.formula_code = 'F_YARN_STAGE_OUT') AS n_edge,
  (SELECT count(*) FROM cost_product_applicable_param capp
     JOIN mst_parameter p ON p.id = capp.capp_param_id AND p.deleted_at IS NULL
    WHERE p.param_code = 'COST_STAGE_OUT') AS n_capp;

-- ============================================================
-- PART 1: the COST_STAGE_OUT param (idempotent)
-- ============================================================
-- Attributes mirror the original 000381:202 definition
--   ('COST_STAGE_OUT','Final Stage Cost (engine sink)','Stage Out','CostOutput',99)
-- which 000381:147-153 inserted as data_type NUMBER / param_category CALCULATED.
-- param_code is VARCHAR(20) (000004_create_mst_parameter.up.sql:4) and
-- 'COST_STAGE_OUT' is 14 characters, so it fits. param_category is constrained to
-- INPUT/RATE/CALCULATED (000004:8) and data_type to NUMBER/TEXT/BOOLEAN (000004:7).
-- uom USD is added here (000381 left the CALCULATED block without a uom); the
-- sink carries a money-per-kg value, consistent with the 000513 params.

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, display_group, display_order,
    is_active, created_at, created_by
)
SELECT
    p.code, p.name, p.short_name, p.data_type, p.category,
    u.uom_id, p.default_val::NUMERIC, p.min_val::NUMERIC, p.max_val::NUMERIC,
    p.display_group, p.display_order, TRUE,
    NOW(), 'seed_stage_out_000515'
FROM (VALUES
  ('COST_STAGE_OUT','Final Stage Cost (engine sink)','Stage Out','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'CostOutput',99)
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val, display_group, display_order)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: the F_YARN_STAGE_OUT formula (idempotent) — after PART 1,
-- result_param_id is resolved from the param seeded above.
-- ============================================================
-- mst_formula.result_param_id is NOT NULL (000005_create_mst_formula.up.sql:8), so the
-- guard below is what keeps this INSERT from erroring if PART 1 somehow wrote nothing.

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT f.code, f.name, f.ftype, f.expr,
       (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1),
       f.descr, 1, TRUE, NOW(), 'seed_stage_out_000515'
FROM (VALUES
  ('F_YARN_STAGE_OUT','Terminal engine sink','CALCULATION','DOMESTIC_COST','COST_STAGE_OUT','Pass-through: COST_STAGE_OUT = DOMESTIC_COST. Feeds ScopeKeyFinalCost (compute.go:41,212) so cpc_cost_per_unit is chosen by design instead of by findTerminalFormula depth + FormulaCode ASC. Source is the legacy TOP 121 Final Ex-Factory Cost. See header FLAG on forwarding inclusion.')
) AS f(code, name, ftype, expr, result_code, descr)
WHERE (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1) IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = f.code AND deleted_at IS NULL
  );

-- ============================================================
-- PART 3: formula_param edge (idempotent) — after PART 2.
-- Drives topo-sort order in the engine: DOMESTIC_COST must be evaluated first.
-- ============================================================

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
  ('F_YARN_STAGE_OUT','DOMESTIC_COST',1)
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
-- PART 4: checklist COST_STAGE_OUT into CAPP for every NON-MB product
-- that already carries DOMESTIC_COST (idempotent)
-- ============================================================
-- loadPerProductFormulas (loader.go:561-576) only returns a formula when the product
-- has a CAPP row for that formula's RESULT param. Seeding the param and the formula
-- alone therefore leaves F_YARN_STAGE_OUT loaded for nobody and branch (a) still dead.
--
-- Scope is DERIVED, never literal: exactly the products that already carry a
-- DOMESTIC_COST CAPP row (written by 000513:178-196), minus MB. There is no
-- product_sys_id literal anywhere in this file.

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT
    src.capp_product_sys_id,
    np.id,
    FALSE, NULL::INT, 'seed_stage_out_000515'
FROM cost_product_applicable_param src
JOIN mst_parameter       sp  ON sp.id  = src.capp_param_id AND sp.deleted_at IS NULL
JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = src.capp_product_sys_id
JOIN cost_product_type   pt  ON pt.cpt_type_id         = cpm.cpm_product_type_id
CROSS JOIN mst_parameter np
WHERE sp.param_code = 'DOMESTIC_COST'
  AND pt.cpt_type_code <> 'MB'
  AND np.param_code = 'COST_STAGE_OUT'
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = src.capp_product_sys_id
        AND capp.capp_param_id       = np.id
  );

-- ============================================================
-- PART 5: HARD VERIFICATION — asserts on ROWS, not on definitions
-- ============================================================

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
    SELECT * INTO pre FROM m515_pre;

    -- ---- param ----
    SELECT count(*) INTO n_param
      FROM mst_parameter
     WHERE param_code = 'COST_STAGE_OUT' AND deleted_at IS NULL;

    -- ---- formula (and that it actually resolved a live result param) ----
    SELECT count(*) INTO n_formula
      FROM mst_formula
     WHERE formula_code = 'F_YARN_STAGE_OUT' AND deleted_at IS NULL;

    SELECT count(*) INTO n_formula_bad_res
      FROM mst_formula f
      LEFT JOIN mst_parameter p ON p.id = f.result_param_id AND p.deleted_at IS NULL
     WHERE f.formula_code = 'F_YARN_STAGE_OUT'
       AND f.deleted_at IS NULL
       AND p.id IS NULL;

    -- ---- formula_param edge: assert the declared edge is present ----
    -- Expectation restated from this file's own PART 3 VALUES list so it can never
    -- drift into "a number someone measured on dev".
    SELECT count(*) INTO n_edge_missing
      FROM (VALUES
        ('F_YARN_STAGE_OUT','DOMESTIC_COST')
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
     WHERE f.formula_code = 'F_YARN_STAGE_OUT';

    -- ---- CAPP: relational expectation, re-derived at runtime ----
    -- Exactly the PART 4 scope: distinct non-MB products carrying DOMESTIC_COST.
    SELECT count(DISTINCT src.capp_product_sys_id) INTO n_base_products
      FROM cost_product_applicable_param src
      JOIN mst_parameter       sp  ON sp.id  = src.capp_param_id AND sp.deleted_at IS NULL
      JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = src.capp_product_sys_id
      JOIN cost_product_type   pt  ON pt.cpt_type_id         = cpm.cpm_product_type_id
     WHERE sp.param_code = 'DOMESTIC_COST'
       AND pt.cpt_type_code <> 'MB';

    SELECT count(*) INTO n_capp
      FROM cost_product_applicable_param capp
      JOIN mst_parameter p ON p.id = capp.capp_param_id AND p.deleted_at IS NULL
     WHERE p.param_code = 'COST_STAGE_OUT';

    n_capp_expected := n_base_products;

    -- ---- UOM resolution (PART 1 uses a LEFT JOIN, so a missing USD row would
    -- ---- silently leave uom_id NULL instead of failing) ----
    SELECT count(*) INTO n_uom_usd FROM mst_uom WHERE uom_code = 'USD' AND deleted_at IS NULL;
    SELECT count(*) INTO n_param_no_uom
      FROM mst_parameter
     WHERE param_code = 'COST_STAGE_OUT' AND deleted_at IS NULL AND uom_id IS NULL;

    RAISE NOTICE '000515: params %->% (this run inserted %), formulas %->% (inserted %), formula_param edges %->% (inserted %), CAPP rows %->% (inserted %)',
        pre.n_param, n_param, n_param - pre.n_param,
        pre.n_formula, n_formula, n_formula - pre.n_formula,
        pre.n_edge, n_edge, n_edge - pre.n_edge,
        pre.n_capp, n_capp, n_capp - pre.n_capp;
    RAISE NOTICE '000515: non-MB product base (products carrying DOMESTIC_COST, excluding cpt_type_code=''MB'') = %, expected COST_STAGE_OUT CAPP rows = %',
        n_base_products, n_capp_expected;

    IF n_param <> 1 THEN
        RAISE EXCEPTION '000515: expected exactly 1 live COST_STAGE_OUT param, found % — PART 1 wrote nothing usable', n_param;
    END IF;

    -- PART 0 already proved exactly one non-deleted USD row exists, so any NULL
    -- uom_id here is a real defect, not a missing-master situation.
    IF n_param_no_uom > 0 THEN
        RAISE EXCEPTION '000515: mst_uom has USD (% row(s)) but COST_STAGE_OUT ended up with NULL uom_id (% row(s))', n_uom_usd, n_param_no_uom;
    END IF;

    IF n_formula <> 1 THEN
        RAISE EXCEPTION '000515: expected exactly 1 live F_YARN_STAGE_OUT formula, found % — PART 2 wrote nothing usable', n_formula;
    END IF;

    IF n_formula_bad_res > 0 THEN
        RAISE EXCEPTION '000515: F_YARN_STAGE_OUT has a result_param_id that resolves to no live param (% row(s))', n_formula_bad_res;
    END IF;

    IF n_edge_missing > 0 THEN
        RAISE EXCEPTION '000515: % declared formula_param edge(s) missing — PART 3 silently filtered rows away; without the DOMESTIC_COST edge the engine cannot topo-sort F_YARN_STAGE_OUT after its input', n_edge_missing;
    END IF;

    -- ---- Is this a real (populated) catalogue, or a migration-only database? ----
    -- The only products any migration ever inserts are the TXFX_% fixtures of
    -- 000236 / 000239, and those fixtures never carry DOMESTIC_COST (000513 derives
    -- its CAPP scope from RM_LANDED_COST / DELIVERY_COST_QLTY_LOSS, which the
    -- fixtures do not have either). So on a database built from migrations alone —
    -- exactly what CI does — n_base_products = 0 is the CORRECT and unavoidable
    -- outcome, and failing there would block the whole migration chain over an
    -- absence of data that is expected by construction. Real products only ever
    -- arrive through the application / Oracle import.
    --
    -- This guard is a BUSINESS-VOLUME assertion, not a correctness one. Correctness
    -- stays unconditional in the shortfall check below: when the base is 0 the
    -- expectation is 0 and 0 >= 0 passes for the right reason rather than by being
    -- skipped.
    SELECT COUNT(*) INTO n_real_products
      FROM cost_product_master
     WHERE cpm_product_code NOT LIKE 'TXFX\_%';

    IF n_real_products > 0 AND n_base_products = 0 THEN
        RAISE EXCEPTION '000515: no non-MB product carries DOMESTIC_COST in cost_product_applicable_param — the COST_STAGE_OUT backfill would write 0 rows, loadPerProductFormulas would never return F_YARN_STAGE_OUT, and cpc_cost_per_unit would keep being picked by the findTerminalFormula heuristic. Run 000513 first.';
    ELSIF n_base_products = 0 THEN
        RAISE NOTICE '000515: no non-MB product carries DOMESTIC_COST (non-fixture products = %) — COST_STAGE_OUT backfill wrote 0 rows. This is EXPECTED on a migration-only database such as CI, where only the TXFX_%% fixtures exist. On production it would mean the engine keeps falling back to the findTerminalFormula heuristic.', n_real_products;
    END IF;

    IF n_capp < n_capp_expected THEN
        RAISE EXCEPTION '000515: CAPP rows for COST_STAGE_OUT = %, expected % (one per non-MB product carrying DOMESTIC_COST)',
            n_capp, n_capp_expected;
    END IF;
END $verify$;

COMMIT;
