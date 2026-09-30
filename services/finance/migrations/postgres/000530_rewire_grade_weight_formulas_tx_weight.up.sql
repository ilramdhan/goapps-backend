-- 000530: rewire the five grade-weight formulas through the tx_weight() built-in.
--
-- Before (000408): F_YARN_<G>_WT = 'AX_WT * <G>_PERC / AX_PERC'
-- After:           F_YARN_<G>_WT = 'tx_weight(''<G>'', AX_WT, AX_WT * <G>_PERC / AX_PERC)'
--
-- tx_weight(grade, axWt, fallback) is injected into every evaluation scope by
-- internal/application/costcalc/tx_weight.go (injectTxWeight). It looks up the
-- product type's live mst_yarn_tx_weight rule (000528/000529) for the grade:
--   LESS_BY  -> AX_WT - value
--   MULTIPLY -> AX_WT * value
--   FIXED    -> value
-- and returns the fallback (the original ratio expression, verbatim) when no
-- rule exists. A product type without rules therefore computes exactly as
-- before this migration.
--
-- Guarded: each UPDATE only fires when the expression still equals the exact
-- 000408 text, so a hand-edited formula is left alone (a NOTICE reports it).
-- Marker updated_by = 'tx_weight_000530' keys the down migration.
-- No formula_param change: AX_WT, <G>_PERC and AX_PERC are already inputs.

BEGIN;

UPDATE mst_formula
SET expression = 'tx_weight(''AE'', AX_WT, AX_WT * AE_PERC / AX_PERC)',
    updated_at = NOW(),
    updated_by = 'tx_weight_000530'
WHERE formula_code = 'F_YARN_AE_WT'
  AND deleted_at IS NULL
  AND expression = 'AX_WT * AE_PERC / AX_PERC';

UPDATE mst_formula
SET expression = 'tx_weight(''A9'', AX_WT, AX_WT * A9_PERC / AX_PERC)',
    updated_at = NOW(),
    updated_by = 'tx_weight_000530'
WHERE formula_code = 'F_YARN_A9_WT'
  AND deleted_at IS NULL
  AND expression = 'AX_WT * A9_PERC / AX_PERC';

UPDATE mst_formula
SET expression = 'tx_weight(''A'', AX_WT, AX_WT * A_PERC / AX_PERC)',
    updated_at = NOW(),
    updated_by = 'tx_weight_000530'
WHERE formula_code = 'F_YARN_A_WT'
  AND deleted_at IS NULL
  AND expression = 'AX_WT * A_PERC / AX_PERC';

UPDATE mst_formula
SET expression = 'tx_weight(''B'', AX_WT, AX_WT * B_PERC / AX_PERC)',
    updated_at = NOW(),
    updated_by = 'tx_weight_000530'
WHERE formula_code = 'F_YARN_B_WT'
  AND deleted_at IS NULL
  AND expression = 'AX_WT * B_PERC / AX_PERC';

UPDATE mst_formula
SET expression = 'tx_weight(''C'', AX_WT, AX_WT * C_PERC / AX_PERC)',
    updated_at = NOW(),
    updated_by = 'tx_weight_000530'
WHERE formula_code = 'F_YARN_C_WT'
  AND deleted_at IS NULL
  AND expression = 'AX_WT * C_PERC / AX_PERC';

DO $$
DECLARE
    v_rewired INT;
BEGIN
    SELECT COUNT(*) INTO v_rewired
    FROM mst_formula
    WHERE formula_code IN ('F_YARN_AE_WT', 'F_YARN_A9_WT', 'F_YARN_A_WT', 'F_YARN_B_WT', 'F_YARN_C_WT')
      AND deleted_at IS NULL
      AND updated_by = 'tx_weight_000530';
    IF v_rewired < 5 THEN
        RAISE NOTICE '000530: rewired % of 5 grade-weight formulas; the rest no longer match the 000408 expression and were left unchanged', v_rewired;
    ELSE
        RAISE NOTICE '000530: rewired all 5 grade-weight formulas through tx_weight()';
    END IF;
END $$;

COMMIT;
