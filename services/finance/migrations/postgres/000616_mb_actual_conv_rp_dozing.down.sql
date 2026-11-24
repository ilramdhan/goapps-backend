-- 000616 down — restore expressions/audit cols from bak_000616_formula only where the formula still has
-- the exact text 000616 wrote; delete only edges 000616 inserted; delete the two params only if 000616
-- created them and nothing references them (cost_product_parameter / CAPP / formula_param / machine defaults).
BEGIN;

DO $$
DECLARE
    r RECORD;
    v_new TEXT;
BEGIN
    IF to_regclass('bak_000616_formula') IS NULL THEN
        RAISE NOTICE '000616 down: bak_000616_formula missing, formulas NOT restored';
    ELSE
        FOR r IN SELECT * FROM bak_000616_formula LOOP
            v_new := CASE r.formula_code
                WHEN 'F_YARN_CONV_FACTOR' THEN 'CONV_FACTOR_ACTUAL > 0 ? CONV_FACTOR_ACTUAL : 1'
                WHEN 'F_YARN_RP_DOZING' THEN 'RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0)'
                WHEN 'F_YARN_MB_COST' THEN 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0))'
            END;
            -- only restore (and drop the edge) when the formula is still exactly what 000616 wrote
            IF EXISTS (SELECT 1 FROM mst_formula WHERE formula_code = r.formula_code AND deleted_at IS NULL AND expression = v_new) THEN
                IF r.edge_inserted THEN
                    DELETE FROM formula_param fp
                    USING mst_formula f, mst_parameter p
                    WHERE fp.formula_id = f.id AND fp.param_id = p.id
                      AND f.formula_code = r.formula_code AND f.deleted_at IS NULL
                      AND p.param_code = r.edge_param_code AND p.deleted_at IS NULL;
                END IF;
                UPDATE mst_formula
                   SET expression = r.old_expression, updated_at = r.old_updated_at, updated_by = r.old_updated_by
                 WHERE formula_code = r.formula_code AND deleted_at IS NULL AND expression = v_new;
            ELSE
                RAISE NOTICE '000616 down: % not restored (expression changed since 000616)', r.formula_code;
            END IF;
        END LOOP;
    END IF;
END $$;

DELETE FROM mst_parameter p
WHERE p.param_code IN ('CONV_FACTOR_ACTUAL', 'RP_DOZING_ACTUAL')
  AND p.created_by = 'migration_000616'
  AND NOT EXISTS (SELECT 1 FROM cost_product_parameter c WHERE c.cpp_param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM cost_product_applicable_param a WHERE a.capp_param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM mst_machine_param_default d WHERE d.mmpd_param_id = p.id);

DROP TABLE IF EXISTS bak_000616_formula;

COMMIT;
