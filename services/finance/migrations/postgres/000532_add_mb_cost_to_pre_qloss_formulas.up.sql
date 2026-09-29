-- Top Check 94 / 95 += MB Cost (Top 73 "MB Cost Marketing").
--
-- Backlog 1, item 2 (docs/superpowers/specs/2026-09-29-backlog1-txweight-top94-
-- rmgroup-design.md). The pre-quality-loss costs seeded by
-- 000408_seed_oracle_formulas.up.sql:41-42 omit the masterbatch cost:
--
--   F_YARN_CAP_PRE_QL (param 94 CAPTIVE_COST_BEFORE_QLOSS)
--     RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB
--   F_YARN_DEL_PRE_QL (param 95 DELIVERY_COST_BEFORE_QLOSS)
--     RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB
--
-- Both get "+ MB_COST_MKT" (produced by F_YARN_MB_COST, 000408:34). No other
-- formula consumes MB_COST_MKT, so there is no double counting. The conversion
-- params are named "..._EXCL_MB" precisely because MB is added at this step.
--
-- IMPACT: BC loss, non-std loss, quality loss and cap/del final costs move for
-- every product with a non-zero MB cost. Already-calculated periods must be
-- recalculated after deploy.
--
-- Pattern mirrors 000510: 000408 is immutable and is NOT edited; the UPDATE
-- only fires when the expression still equals the exact 000408 text (a hand-
-- edited formula is left alone), rows are stamped with the marker
-- 'add_mb_cost_000532', and the formula_param edges are NOT EXISTS-guarded.
-- Re-running is a no-op. A NOTICE reports the affected row counts so the
-- deploy can confirm both formulas were rewritten (expect 2 / 2).

BEGIN;

DO $$
DECLARE
    v_updated  INTEGER;
    v_inserted INTEGER;
BEGIN
    -- Never reference a param that does not exist: the engine would zero-fill
    -- the unknown identifier instead of failing loudly (compute.go,
    -- buildInitialScope).
    IF NOT EXISTS (
        SELECT 1 FROM mst_parameter
        WHERE param_code = 'MB_COST_MKT'
          AND deleted_at IS NULL
    ) THEN
        RAISE EXCEPTION '000532: parameter MB_COST_MKT not found; cannot add MB cost to F_YARN_CAP_PRE_QL / F_YARN_DEL_PRE_QL';
    END IF;

    -- ------------------------------------------------------------
    -- PART 1: Append the MB_COST_MKT term to both expressions
    -- ------------------------------------------------------------
    UPDATE mst_formula f
    SET expression = CASE f.formula_code
            WHEN 'F_YARN_CAP_PRE_QL' THEN 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB + MB_COST_MKT'
            WHEN 'F_YARN_DEL_PRE_QL' THEN 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB + MB_COST_MKT'
        END,
        updated_at = NOW(),
        updated_by = 'add_mb_cost_000532'
    WHERE f.deleted_at IS NULL
      AND (
          (f.formula_code = 'F_YARN_CAP_PRE_QL'
           AND f.expression = 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB'
           AND EXISTS (SELECT 1 FROM mst_parameter p
                       WHERE p.id = f.result_param_id
                         AND p.param_code = 'CAPTIVE_COST_BEFORE_QLOSS'
                         AND p.deleted_at IS NULL))
       OR (f.formula_code = 'F_YARN_DEL_PRE_QL'
           AND f.expression = 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB'
           AND EXISTS (SELECT 1 FROM mst_parameter p
                       WHERE p.id = f.result_param_id
                         AND p.param_code = 'DELIVERY_COST_BEFORE_QLOSS'
                         AND p.deleted_at IS NULL))
      );
    GET DIAGNOSTICS v_updated = ROW_COUNT;

    -- ------------------------------------------------------------
    -- PART 2: Declare MB_COST_MKT as the 4th input edge
    -- ------------------------------------------------------------
    -- sort_order 4 continues 000408:163,165 (RM_NORMS 1, RM_LANDED_COST 2,
    -- ONLY_CONV_*_PACK_EXCL_MB 3). Only formulas this migration rewrote are
    -- linked, so the edge and the expression cannot drift apart.
    INSERT INTO formula_param (formula_id, param_id, sort_order)
    SELECT f.id, p.id, 4
    FROM mst_formula f
    JOIN mst_parameter p
      ON p.param_code = 'MB_COST_MKT'
     AND p.deleted_at IS NULL
    WHERE f.formula_code IN ('F_YARN_CAP_PRE_QL', 'F_YARN_DEL_PRE_QL')
      AND f.deleted_at IS NULL
      AND f.updated_by = 'add_mb_cost_000532'
      AND NOT EXISTS (
          SELECT 1 FROM formula_param fp
          WHERE fp.formula_id = f.id
            AND fp.param_id = p.id
      );
    GET DIAGNOSTICS v_inserted = ROW_COUNT;

    RAISE NOTICE '000532: % pre-quality-loss formula(s) updated (expect 2), % MB_COST_MKT formula_param edge(s) inserted (expect 2)',
        v_updated, v_inserted;
END $$;

COMMIT;
