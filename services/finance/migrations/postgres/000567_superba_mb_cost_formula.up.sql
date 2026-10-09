-- 000567 — F_YARN_MB_COST (result MB_COST_MKT, TOP 73): SUPERBA products take the
-- Superba Cost SP master value (SUPERBA_MB_COST, engine-injected from old_value);
-- every other product keeps the original expression.
--
--   before: MB_RATE_MKT * MB_SP_DOZING / 100.0                         (000408:34, never rewritten since)
--   after : IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)
--
-- *** DO NOT APPLY before the coverage SQL (plan P9-T2) has been run: every SUPERBA
-- *** product whose normalised shade has no active cost_superba_cost_sp row becomes
-- *** BLOCKED / MISSING_SUPERBA_COST after this migration.
--
-- IS_SUPERBA and SUPERBA_MB_COST are reserved engine keys (like IS_POY / IS_ACTUAL):
-- NOT mst_parameter rows, no formula_param edge (compute.go applySuperbaMBCost).
-- Guarded by the exact current text: a hand-edited formula is left alone, and a
-- NOTICE (not an exception) reports 0 rows so prod drift is visible without failing.
BEGIN;

DO $$
DECLARE
    v_updated INTEGER;
BEGIN
    UPDATE mst_formula f
    SET expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)',
        updated_at = NOW(),
        updated_by = 'superba_mb_cost_000567'
    WHERE f.formula_code = 'F_YARN_MB_COST'
      AND f.deleted_at IS NULL
      AND f.expression = 'MB_RATE_MKT * MB_SP_DOZING / 100.0'
      AND EXISTS (SELECT 1 FROM mst_parameter p
                  WHERE p.id = f.result_param_id
                    AND p.param_code = 'MB_COST_MKT'
                    AND p.deleted_at IS NULL);
    GET DIAGNOSTICS v_updated = ROW_COUNT;

    IF v_updated = 0 THEN
        RAISE NOTICE '000567: F_YARN_MB_COST not rewritten (0 rows): expression drifted from the 000408 text, or already migrated. Inspect mst_formula manually.';
    ELSE
        RAISE NOTICE '000567: F_YARN_MB_COST rewritten (% row)', v_updated;
    END IF;
END $$;

COMMIT;
