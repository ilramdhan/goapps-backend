-- 000568 down — restore the exact pre-migration expression/audit columns from bak_000568_formula,
-- only where the formula still carries the 000568 marker and the expression 000568 wrote.
-- The WASTE_PERC edge is deleted only if 000568 inserted it (edge_inserted). Then the backup is dropped.
BEGIN;

DO $$
DECLARE
    v_restored INTEGER := 0;
BEGIN
    IF to_regclass('bak_000568_formula') IS NULL THEN
        RAISE NOTICE '000568 down: bak_000568_formula missing, formula NOT restored';
        RETURN;
    END IF;

    DELETE FROM formula_param fp
    USING mst_formula f, mst_parameter p, bak_000568_formula b
    WHERE fp.formula_id = f.id AND fp.param_id = p.id
      AND f.formula_code = 'F_YARN_MB_COST' AND b.formula_code = f.formula_code
      AND f.deleted_at IS NULL AND f.updated_by = 'superba_mb_cost_000568'
      AND p.param_code = 'WASTE_PERC' AND b.edge_inserted;

    UPDATE mst_formula f
       SET expression = b.old_expression,
           updated_at = b.old_updated_at,
           updated_by = b.old_updated_by
      FROM bak_000568_formula b
     WHERE f.formula_code = b.formula_code AND f.deleted_at IS NULL
       AND f.updated_by = 'superba_mb_cost_000568'
       AND f.expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)';
    GET DIAGNOSTICS v_restored = ROW_COUNT;
    IF v_restored = 0 THEN
        RAISE NOTICE '000568 down: nothing restored (marker/expression not found)';
    END IF;
END $$;

DROP TABLE IF EXISTS bak_000568_formula;

COMMIT;
