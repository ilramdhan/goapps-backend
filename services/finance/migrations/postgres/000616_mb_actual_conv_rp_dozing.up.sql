-- 000616 — MB comparison P1: actual CONV_FACTOR / RP_DOZING as INPUT params with fallback.
-- Spec: docs/superpowers/specs/2026-11-24-mb-comparison-p1-design.md §2 (rule A, user 2026-11-24).
-- New INPUT params CONV_FACTOR_ACTUAL, RP_DOZING_ACTUAL (filled per product by the mb-comparison-fill tool).
-- Each formula is rewritten ONLY when its expression equals EXACTLY the current prod text; the old text stays
-- as the fallback arm when the actual input is 0/empty, so products without actuals compute identically.
-- Untouched: F_YARN_MB_FLAG, F_YARN_WASTE_LESS_MB_OPU, SUPERBA branch, mst_machine_param_default (000614).
-- Backup bak_000616_formula holds old text/audit cols + whether THIS migration inserted the formula_param edge.
-- Params inserted here carry created_by = 'migration_000616' (down removes only those, and only if unreferenced).
BEGIN;

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, display_group, display_order, created_at, created_by
)
SELECT v.code, v.name, v.name, 'NUMBER', 'INPUT', NULL, TRUE, 'Masterbatch & SP', v.ord, NOW(), 'migration_000616'
FROM (VALUES
    ('CONV_FACTOR_ACTUAL', 'Conv Factor (Actual)', 72),
    ('RP_DOZING_ACTUAL',   'RP Dozing (Actual)',   73)
) AS v(code, name, ord)
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS bak_000616_formula (
    formula_code    VARCHAR(100) PRIMARY KEY,
    old_expression  TEXT NOT NULL,
    old_updated_at  TIMESTAMPTZ,
    old_updated_by  VARCHAR(100),
    edge_param_code VARCHAR(50) NOT NULL,
    edge_inserted   BOOLEAN NOT NULL DEFAULT FALSE,
    backed_up_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE bak_000616_formula IS 'Backup written by migration 000616; read by its down migration.';

DO $$
DECLARE
    r       RECORD;
    v_n     INTEGER;
BEGIN
    FOR r IN SELECT * FROM (VALUES
        ('F_YARN_CONV_FACTOR', 'CONV_FACTOR_ACTUAL',
         '1',
         'CONV_FACTOR_ACTUAL > 0 ? CONV_FACTOR_ACTUAL : 1'),
        ('F_YARN_RP_DOZING', 'RP_DOZING_ACTUAL',
         'MB_SP_DOZING > 0 ? MB_SP_DOZING : 0',
         'RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0)'),
        ('F_YARN_MB_COST', 'RP_DOZING_ACTUAL',
         'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)',
         'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0))')
    ) AS t(fcode, pcode, old_expr, new_expr)
    LOOP
        INSERT INTO bak_000616_formula (formula_code, old_expression, old_updated_at, old_updated_by, edge_param_code, edge_inserted)
        SELECT f.formula_code, f.expression, f.updated_at, f.updated_by, r.pcode,
               NOT EXISTS (SELECT 1 FROM formula_param fp
                           JOIN mst_parameter p ON p.id = fp.param_id AND p.param_code = r.pcode AND p.deleted_at IS NULL
                           WHERE fp.formula_id = f.id)
        FROM mst_formula f
        WHERE f.formula_code = r.fcode AND f.deleted_at IS NULL AND f.expression = r.old_expr
        ON CONFLICT (formula_code) DO NOTHING;

        UPDATE mst_formula
           SET expression = r.new_expr, updated_at = NOW(), updated_by = 'migration_000616'
         WHERE formula_code = r.fcode AND deleted_at IS NULL AND expression = r.old_expr;
        GET DIAGNOSTICS v_n = ROW_COUNT;

        IF v_n = 0 THEN
            RAISE NOTICE '000616: % not rewritten (0 rows): expression drifted from the known prod text, or already migrated.', r.fcode;
        ELSE
            INSERT INTO formula_param (formula_id, param_id, sort_order)
            SELECT f.id, p.id,
                   COALESCE((SELECT MAX(fp2.sort_order) FROM formula_param fp2 WHERE fp2.formula_id = f.id), 0) + 1
            FROM mst_formula f
            JOIN mst_parameter p ON p.param_code = r.pcode AND p.deleted_at IS NULL
            WHERE f.formula_code = r.fcode AND f.deleted_at IS NULL
            ON CONFLICT DO NOTHING;
        END IF;
    END LOOP;
END $$;

COMMIT;
