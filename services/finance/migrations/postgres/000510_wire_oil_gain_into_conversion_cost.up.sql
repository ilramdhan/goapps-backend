-- Wire OIL_GAIN into the yarn conversion build-up: add it as a term of
-- F_YARN_CONV_CAP / F_YARN_CONV_DEL and declare it as their 6th input.
--
-- WHY THIS MIGRATION EXISTS
-- 000408_seed_oracle_formulas.up.sql:39-40 seeds the two conversion formulas
-- WITHOUT an OIL_GAIN term, even though OIL_GAIN is a real per-product cost
-- component (mst_parameter seeded at 000407_seed_oracle_142_params.up.sql:102,
-- printed as CSV row 39 "Oil Gain." at internal/worker/costsheet_rows.go:179 and
-- as all-data column "125.OIL GAIN" at internal/worker/costsheet_alldata.go:194).
-- The omission is a confirmed defect, not a design choice:
--
--   * docs/export-product-cost/row81-del-pre-ql-analysis.md:85-92 fits 11
--     hypotheses against two independent reference data points. H1 (the seeded
--     expression, no OIL_GAIN) leaves residuals -0.00065 / -0.03665 and FAILS.
--     H2 (seeded + OIL_GAIN) leaves -0.00065 / -0.00065 and FITS. H3 (seeded +
--     OIL_GAIN + DUTY) FAILS. So the missing term is OIL_GAIN and nothing else.
--   * docs/export-product-cost/row81-del-pre-ql-analysis.md:117 states the
--     additive reference form explicitly:
--     RM_NORMS*RM_LANDED + TFC + DelPack + OilCost + OilGain + Interm + Spl1.
--   * docs/export-product-cost/impact-trace-verified.md:288-295 classifies the
--     missing term as "B (cacat kode) ... milik IT" — an IT-side code defect
--     that needs no costing-team input — and impact-trace-verified.md:318 lists
--     it as action item 4, explicitly requiring a NEW migration because 000408
--     is immutable after merge. This file is that migration.
--
-- SIGN OF THE TERM: the operator is PLUS, not MINUS.
-- OIL_GAIN is a "gain" by name but is stored as an ALREADY-NEGATIVE number, so
-- the sign lives in the data and the formula must simply add it. Evidence:
--   * <repo-root>/data-examples/import-file-csv/param_value_import/
--     product_parameters_1.csv:101 — OIL_GAIN = -0.0495 (and identically at
--     :201, :301, ... for every product in that import file).
--   * <repo-root>/data-examples/export-product-cost/example-export-param.txt:39
--     — reference workbook prints "39.Oil Gain." as -0,002 and -0,036.
--   * impact-trace-verified.md:290-292 records the same reading: the reference
--     file shows Oil Gain "tidak nol dan bernilai negatif (-0,002; -0,035;
--     -0,036)".
--   * The residual fit above only works with PLUS: H2 adds the (negative) value
--     and lands on target; subtracting it would double the error.
-- Writing '- OIL_GAIN' here would therefore INCREASE conversion cost by the oil
-- gain instead of reducing it — the opposite of the intended behavior.
--
-- (!) OPEN QUESTION — NEEDS COSTING-TEAM CONFIRMATION, DO NOT TREAT AS SETTLED.
-- The COEFFICIENT is assumed to be exactly +1. Two data points are not enough to
-- separate +1 from a near-1 scale factor; this is recorded verbatim as open
-- question 4 in docs/export-product-cost/impact-trace-verified.md:398 ("Apakah
-- koefisien OIL_GAIN persis +1. Dua titik data tidak cukup memisahkan"), and is
-- also priority 3 of the costing-team queue (impact-trace-verified.md:333). The
-- SIGN is confirmed; the MAGNITUDE multiplier is assumed. If costing returns a
-- coefficient other than 1, supersede this migration with a new one.
--
-- BLAST RADIUS: ZERO TODAY.
-- F_YARN_OIL_GAIN is seeded with the literal expression '0' (000408:28,
-- described there as "Always 0"), so the added term evaluates to 0 for every
-- product and no computed number moves. This migration fixes the STRUCTURE; the
-- value only becomes non-zero when F_YARN_OIL_GAIN itself is given a real rule,
-- which is the separate costing-gated item D (impact-trace-verified.md:294-295).
-- Do NOT connect CSV row 81 on the strength of this migration alone.
--
-- TERMINAL-SELECTION SAFETY.
-- findTerminalFormula (internal/application/costcalc/compute.go:810-873) picks
-- the final cost as the unconsumed result param with the deepest formula chain,
-- tie-broken by FormulaCode ASC. OIL_GAIN is today an unconsumed depth-1 SINK,
-- so it sits in the tie-break candidate set. Consuming it REMOVES a stray
-- candidate and cannot raise the depth of CONV_CAP/CONV_DEL (their depth is
-- driven by TOTAL_FIXEDCOST_PER_KG, a much longer chain). This is locked by the
-- database-free regression test
-- internal/application/costcalc/compute_terminal_oil_gain_test.go, which asserts
-- both that OIL_GAIN leaves the terminal set and that the selected terminal
-- formula and the resulting CostPerUnit are unchanged.
--
-- 000408 is immutable and is NOT edited. This migration runs after it.
-- Every statement is guarded: on a database missing the formulas or the OIL_GAIN
-- param, each statement matches nothing and the migration is a safe no-op.
-- Re-running is a no-op (the expression guards require the pre-change text, and
-- the formula_param inserts are NOT EXISTS-guarded).

BEGIN;

-- ============================================================
-- PART 1: Append the OIL_GAIN term to both expressions
-- ============================================================
-- The WHERE clause pins the EXACT expression seeded by 000408. That makes the
-- statement idempotent (after it runs, the text no longer matches) and, more
-- importantly, refuses to touch a formula whose expression somebody has already
-- changed by hand or via a later migration — better a visible no-op than a
-- silent clobber. The result param is checked too, so a future formula that
-- reuses the code for a different result is not rewritten by this migration.

UPDATE mst_formula f
SET expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'wire_oil_gain_000510'
WHERE f.formula_code = 'F_YARN_CONV_CAP'
  AND f.deleted_at IS NULL
  AND f.expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id
        AND p.param_code = 'ONLY_CONV_CAP_PACK_EXCL_MB'
        AND p.deleted_at IS NULL
  )
  -- Guard: never reference a param that does not exist, or the engine would
  -- zero-fill an unknown identifier instead of failing loudly
  -- (internal/application/costcalc/compute.go, buildInitialScope).
  AND EXISTS (
      SELECT 1 FROM mst_parameter op
      WHERE op.param_code = 'OIL_GAIN'
        AND op.deleted_at IS NULL
  );

UPDATE mst_formula f
SET expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN',
    updated_at = NOW(),
    updated_by = 'wire_oil_gain_000510'
WHERE f.formula_code = 'F_YARN_CONV_DEL'
  AND f.deleted_at IS NULL
  AND f.expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1'
  AND EXISTS (
      SELECT 1 FROM mst_parameter p
      WHERE p.id = f.result_param_id
        AND p.param_code = 'ONLY_CONV_DEL_PACK_EXCL_MB'
        AND p.deleted_at IS NULL
  )
  AND EXISTS (
      SELECT 1 FROM mst_parameter op
      WHERE op.param_code = 'OIL_GAIN'
        AND op.deleted_at IS NULL
  );

-- ============================================================
-- PART 2: Declare OIL_GAIN as an input edge of both formulas
-- ============================================================
-- formula_param is the formula -> input edge table (000005_create_mst_formula
-- .up.sql:47-52). It is what loadPerProductFormulas reads to populate
-- Formula.InputParamCodes, and therefore what findTerminalFormula uses to decide
-- which result params are consumed. Without these two rows the expression change
-- alone would leave OIL_GAIN looking like an unconsumed sink.
--
-- sort_order 6 continues the 1-based sequence 000408 used for these two formulas
-- (000408:159 and 000408:161 both end at SPECIAL_COST_1 with sort_order 5).
--
-- The NOT EXISTS guard mirrors the idempotency pattern used throughout this
-- tree (for example 000382_seed_oracle_yarn_formulas.up.sql:312-313) and is
-- also what keeps the statement from violating idx_formula_param_unique
-- (000005:55-56). Only formulas this migration actually rewrote are linked, so
-- the edge and the expression can never drift apart.

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 6
FROM mst_formula f
JOIN mst_parameter p
  ON p.param_code = 'OIL_GAIN'
 AND p.deleted_at IS NULL
WHERE f.formula_code IN ('F_YARN_CONV_CAP', 'F_YARN_CONV_DEL')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'wire_oil_gain_000510'
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp
      WHERE fp.formula_id = f.id
        AND fp.param_id = p.id
  );

COMMIT;
