-- 000567 down — restore the exact text 000567 matched (000408:34), only where the
-- formula still carries the 000567 marker and the expression is the one 000567 wrote.
BEGIN;

DO $$
DECLARE
    v_restored INTEGER;
BEGIN
    UPDATE mst_formula f
    SET expression = 'MB_RATE_MKT * MB_SP_DOZING / 100.0',
        updated_at = NOW(),
        updated_by = NULL
    WHERE f.formula_code = 'F_YARN_MB_COST'
      AND f.deleted_at IS NULL
      AND f.updated_by = 'superba_mb_cost_000567'
      AND f.expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)';
    GET DIAGNOSTICS v_restored = ROW_COUNT;
    IF v_restored = 0 THEN
        RAISE NOTICE '000567 down: nothing restored (marker/expression not found)';
    END IF;
END $$;

COMMIT;
