-- 000618 down — restore expressions/audit cols from bak_000618_formula only where the formula still has the exact
-- text 000618 wrote; delete only edges 000618 inserted; delete the param only if 000618 created it and nothing
-- references it (cost_product_parameter / CAPP / formula_param / machine defaults).
BEGIN;

DO $$
DECLARE
    r RECORD;
    v_new TEXT;
BEGIN
    IF to_regclass('bak_000618_formula') IS NULL THEN
        RAISE NOTICE '000618 down: bak_000618_formula missing, formulas NOT restored';
    ELSE
        FOR r IN SELECT * FROM bak_000618_formula LOOP
            v_new := CASE r.formula_code
                WHEN 'F_YARN_RP_DOZING' THEN 'RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0))'
                WHEN 'F_YARN_MB_COST' THEN 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * (MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING)) * MB_RATE_MKT / 100.0))'
                WHEN 'F_YARN_MB_FLAG' THEN '(MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING) > 0 ? ''Y'' : ''N'''
            END;
            IF EXISTS (SELECT 1 FROM mst_formula WHERE formula_code = r.formula_code AND deleted_at IS NULL AND expression = v_new) THEN
                IF r.edge_inserted THEN
                    DELETE FROM formula_param fp
                    USING mst_formula f, mst_parameter p
                    WHERE fp.formula_id = f.id AND fp.param_id = p.id
                      AND f.formula_code = r.formula_code AND f.deleted_at IS NULL
                      AND p.param_code = 'MB_LDR_OVERRIDE' AND p.deleted_at IS NULL;
                END IF;
                UPDATE mst_formula
                   SET expression = r.old_expression, updated_at = r.old_updated_at, updated_by = r.old_updated_by
                 WHERE formula_code = r.formula_code AND deleted_at IS NULL AND expression = v_new;
            ELSE
                RAISE NOTICE '000618 down: % not restored (expression changed since 000618)', r.formula_code;
            END IF;
        END LOOP;
    END IF;
END $$;

DELETE FROM mst_parameter p
WHERE p.param_code = 'MB_LDR_OVERRIDE'
  AND p.created_by = 'migration_000618'
  AND NOT EXISTS (SELECT 1 FROM cost_product_parameter c WHERE c.cpp_param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM cost_product_applicable_param a WHERE a.capp_param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.param_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM mst_machine_param_default d WHERE d.mmpd_param_id = p.id);

DROP TABLE IF EXISTS bak_000618_formula;

COMMIT;
