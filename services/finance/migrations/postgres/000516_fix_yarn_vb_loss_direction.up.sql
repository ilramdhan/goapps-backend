-- 000516_fix_yarn_vb_loss_direction.up.sql
-- Fix the DIRECTION of the VBx_LOSS change-over formula: divide -> multiply.
--
-- WHAT 000465 DID, AND WHERE IT WENT WRONG
-- 000465_fix_yarn_vb_loss_formula.up.sql set the production expression to
--     VOLUME_BUCKET_n_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST) / (VOLUME_BUCKET_n_QTY * 1000) : 0
-- It correctly introduced RM_LANDED_COST (turning a kg figure into a USD/kg figure),
-- and that part of 000465 is kept. What it got wrong is the OPERATOR on
-- VOLUME_BUCKET_n_QTY.
--
-- The comment at 000465 line 5 states:
--     "VOLUME_BUCKET_x_QTY is a batch-weight threshold in MT (e.g. 0.1923 MT)"
-- ^^ THAT COMMENT IS WRONG, and this migration exists to correct it. 0.1923 is not
-- a tonnage; it is 1/5.2. The source columns mst_machine.vb1_qty..vb5_qty store the
-- RECIPROCAL of the batch tonnage (1/MT), not the tonnage itself.
--
-- EVIDENCE FOR THE RECIPROCAL READING
--   (a) Seed 000424_seed_machine_from_oracle_csv.up.sql carries 37 machines with a
--       complete vb1..vb5 set. For 34 of those 37, 1/vbN lands on a clean multiple
--       of 0.1, e.g. [0.2564,0.1282,0.0855,0.0333,0.0167] -> [3.9,7.8,11.7,30.0,60.0]
--       — a 3.9 / 7.8 / 11.7 progression plus 30 / 60. The stored values themselves
--       form no pattern at all.
--   (b) Legacy Oracle data (5 products): K = L_n / QTY_n is CONSTANT across all five
--       buckets (spread 0.0002-0.0035, i.e. 4-decimal rounding noise). A constant
--       L_n / QTY_n is the signature of a MULTIPLY (L_n = K * QTY_n). Under a divide,
--       L_n * QTY_n would be the constant instead.
--   (c) Unit check, product 20210900891: legacy L = [.0997,.0499,.0333,.0130,.0065],
--       kgs_lost_change = 400, implied RM_LANDED_COST = 0.9738 USD/kg.
--           400 kg * 0.9738 USD/kg / 3.9 MT / 1000 kg/MT = 0.09970  vs legacy .0997
--       Expressed directly in the STORED column value (0.2564 = 1/3.9), the same
--       arithmetic is a multiplication:
--           400 * 0.9738 * 0.2564 / 1000 = 0.09988
--
-- MAGNITUDE OF THE DEFECT BEING FIXED
-- Divide-form vs legacy, per bucket: VB1 ~15x, VB2 ~61x, VB3 ~137x, VB4 ~899x,
-- VB5 ~3596x too large. The ratio is 1/QTY^2, exactly as a flipped operator predicts.
-- This is not cosmetic: F_YARN_VBn_DEL (000408_seed_oracle_formulas.up.sql:55-59,
-- untouched by 000465) is
--     VOLUME_BUCKET_n_DEL_COST = DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_n_LOSS
-- so the error is added straight onto the delivery cost of every bucket.
--
-- WHAT THIS FILE CHANGES
--   mst_formula.expression for F_YARN_VB1_LOSS .. F_YARN_VB5_LOSS, and nothing else.
--
-- WHAT THIS FILE DELIBERATELY DOES NOT CHANGE
--   * 000465 itself. Historical migrations are never edited.
--   * formula_param. The RM_LANDED_COST edge that 000465 added (sort_order 0) is
--     still required by the multiply form, so it stays exactly as it is — in this
--     file and in the .down.sql.
--   * F_YARN_VBn_DEL. Its structure is fine; only its input improves.
--   * Any stored cst_product_cost row. This migration changes the MODEL, not the
--     persisted numbers. A recalculation is required for the correction to appear
--     in cpc_param_snapshot / VOLUME_BUCKET_n_DEL_COST.
--
-- ⚠ FLAG — NOT CONFIRMED BEFORE RUNNING THIS
-- The reciprocal reading is proven against the 000424 SEED (37 machines) and against
-- 5 legacy Oracle products. It has NOT been measured on the production
-- cpc_param_snapshot population. Run docs/superpowers/sql/v-19-dampak-arah-vb-loss-postgres.sql
-- (read-only) FIRST. Block V19-A in particular confirms that production really is
-- running the 000465 divide form — if it is not, the .down.sql of this migration
-- would restore the 000465 expression rather than whatever production actually had.
--
-- ⚠ FLAG — 3 of the 37 seeded machines do NOT yield a clean 0.1 multiple under 1/vbN.
-- Whether that is dirty data or a second convention has not been investigated.
--
-- ⚠ NOT DECIDED HERE: whether the engine's terminal expression (000515) should be
-- DOMESTIC_COST or VOLUME_BUCKET_4_DEL_COST. That is a costing-team decision. It only
-- matters for BLAST RADIUS: under a VOLUME_BUCKET_4_DEL_COST terminal this correction
-- moves cpc_cost_per_unit for every non-MB product; under DOMESTIC_COST it only moves
-- the VB rows of the cost sheet.
--
-- VERIFICATION POLICY (E-9: no silent-success migrations)
-- The guard below uses RAISE EXCEPTION, not RAISE NOTICE, and it asserts on the
-- number of rows that actually CARRY THE NEW EXPRESSION — not on the number of
-- statements this file contains. A soft-deleted or renamed formula therefore aborts
-- the migration instead of leaving 3 of 5 buckets corrected.

BEGIN;

-- ============================================================
-- PART 0: pre-state snapshot, so the closing NOTICE can report what THIS run moved
-- ============================================================
CREATE TEMP TABLE m516_pre ON COMMIT DROP AS
SELECT
  (SELECT count(*) FROM mst_formula
    WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
      AND deleted_at IS NULL) AS n_live,
  (SELECT count(*) FROM mst_formula
    WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
      AND deleted_at IS NULL
      AND expression LIKE '%) / (VOLUME_BUCKET%') AS n_divide_form;

-- ============================================================
-- PART 1: flip the operator on all 5 VBx_LOSS formulas
-- ============================================================
-- Form:  (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_n_QTY) / 1000
-- Units: kg * (USD/kg) * (1/MT) / (kg/MT) = USD/kg — a per-kg cost, as before.
-- The `VOLUME_BUCKET_n_QTY > 0` ternary guard is retained verbatim from 000465:
-- with a reciprocal column, a stored 0 still means "no volume bucket configured",
-- so the guard keeps its original meaning.

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_1_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_1_QTY) / 1000 : 0',
    updated_at = NOW(), updated_by = 'migration_000516'
WHERE formula_code = 'F_YARN_VB1_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_2_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_2_QTY) / 1000 : 0',
    updated_at = NOW(), updated_by = 'migration_000516'
WHERE formula_code = 'F_YARN_VB2_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_3_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_3_QTY) / 1000 : 0',
    updated_at = NOW(), updated_by = 'migration_000516'
WHERE formula_code = 'F_YARN_VB3_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_4_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_4_QTY) / 1000 : 0',
    updated_at = NOW(), updated_by = 'migration_000516'
WHERE formula_code = 'F_YARN_VB4_LOSS' AND deleted_at IS NULL;

UPDATE mst_formula
SET expression = 'VOLUME_BUCKET_5_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_5_QTY) / 1000 : 0',
    updated_at = NOW(), updated_by = 'migration_000516'
WHERE formula_code = 'F_YARN_VB5_LOSS' AND deleted_at IS NULL;

-- ============================================================
-- PART 2: HARD VERIFICATION — RAISE EXCEPTION, never a bare NOTICE
-- ============================================================
-- Asserts on the number of live formulas that actually carry the expected NEW
-- expression, re-derived here rather than trusting that PART 1 ran. Anything other
-- than exactly 5 aborts the transaction.

DO $verify$
DECLARE
    pre           RECORD;
    n_live        INTEGER;
    n_new_form    INTEGER;
    n_old_form    INTEGER;
    n_missing     INTEGER;
    n_landed_edge INTEGER;
    v_codes       TEXT;
BEGIN
    SELECT * INTO pre FROM m516_pre;

    -- How many of the 5 formulas exist at all (not soft-deleted)?
    SELECT count(*) INTO n_live
      FROM mst_formula
     WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
       AND deleted_at IS NULL;

    -- How many carry the exact expected multiply expression? The expectation is
    -- rebuilt from a VALUES list so it can never drift from what PART 1 wrote.
    SELECT count(*) INTO n_new_form
      FROM (VALUES
        ('F_YARN_VB1_LOSS','VOLUME_BUCKET_1_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_1_QTY) / 1000 : 0'),
        ('F_YARN_VB2_LOSS','VOLUME_BUCKET_2_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_2_QTY) / 1000 : 0'),
        ('F_YARN_VB3_LOSS','VOLUME_BUCKET_3_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_3_QTY) / 1000 : 0'),
        ('F_YARN_VB4_LOSS','VOLUME_BUCKET_4_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_4_QTY) / 1000 : 0'),
        ('F_YARN_VB5_LOSS','VOLUME_BUCKET_5_QTY > 0 ? (CHANGE_OVER_QLTY_LOSS * RM_LANDED_COST * VOLUME_BUCKET_5_QTY) / 1000 : 0')
      ) AS want(fcode, expr)
     WHERE EXISTS (
        SELECT 1 FROM mst_formula f
         WHERE f.formula_code = want.fcode
           AND f.deleted_at IS NULL
           AND f.expression = want.expr
     );

    n_missing := 5 - n_new_form;

    -- Any leftover divide-form rows would mean a partial correction.
    SELECT count(*) INTO n_old_form
      FROM mst_formula
     WHERE formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
       AND deleted_at IS NULL
       AND expression LIKE '%) / (VOLUME_BUCKET%';

    -- The multiply form still consumes RM_LANDED_COST, so the 000465 edge must exist
    -- on all 5 formulas or the engine cannot resolve the symbol.
    SELECT count(*) INTO n_landed_edge
      FROM formula_param fp
      JOIN mst_formula   f ON f.id = fp.formula_id AND f.deleted_at IS NULL
      JOIN mst_parameter p ON p.id = fp.param_id   AND p.deleted_at IS NULL
     WHERE f.formula_code IN ('F_YARN_VB1_LOSS','F_YARN_VB2_LOSS','F_YARN_VB3_LOSS','F_YARN_VB4_LOSS','F_YARN_VB5_LOSS')
       AND p.param_code = 'RM_LANDED_COST';

    RAISE NOTICE '000516: live VBx_LOSS formulas = %, divide-form before = %, multiply-form after = %, divide-form remaining = %, RM_LANDED_COST edges = %',
        n_live, pre.n_divide_form, n_new_form, n_old_form, n_landed_edge;

    IF n_live <> 5 THEN
        SELECT string_agg(want.fcode, ', ' ORDER BY want.fcode) INTO v_codes
          FROM (VALUES ('F_YARN_VB1_LOSS'),('F_YARN_VB2_LOSS'),('F_YARN_VB3_LOSS'),('F_YARN_VB4_LOSS'),('F_YARN_VB5_LOSS')) AS want(fcode)
         WHERE NOT EXISTS (
            SELECT 1 FROM mst_formula f WHERE f.formula_code = want.fcode AND f.deleted_at IS NULL
         );
        RAISE EXCEPTION '000516: expected 5 live VBx_LOSS formulas, found %. Missing / soft-deleted: %. Refusing to correct a subset of the buckets.', n_live, COALESCE(v_codes, '(none)');
    END IF;

    IF n_new_form <> 5 THEN
        RAISE EXCEPTION '000516: only % of 5 VBx_LOSS formulas carry the corrected multiply expression (% still missing it, % still on the 000465 divide form). A partial correction would leave buckets inconsistent with each other — aborting.', n_new_form, n_missing, n_old_form;
    END IF;

    IF n_old_form > 0 THEN
        RAISE EXCEPTION '000516: % VBx_LOSS formula(s) still match the 000465 divide form after the UPDATEs — aborting.', n_old_form;
    END IF;

    IF n_landed_edge <> 5 THEN
        RAISE EXCEPTION '000516: expected an RM_LANDED_COST formula_param edge on all 5 VBx_LOSS formulas (added by 000465 PART 2), found %. The multiply expression reads RM_LANDED_COST, so without the edge the engine cannot topo-sort it. Run 000465 first.', n_landed_edge;
    END IF;
END $verify$;

COMMIT;
