-- 000618 — MB comparison P2 item 3: per-product LDR override (INPUT param MB_LDR_OVERRIDE).
-- Spec: docs/superpowers/specs/2026-11-26-mb-comparison-p2-item3-override.md (user decisions 2026-11-26).
-- Precedence: RP_DOZING_ACTUAL (000616) > MB_LDR_OVERRIDE > MB_SP_DOZING. Override = actual LDR used as-is (no conv factor).
-- Each of F_YARN_RP_DOZING / F_YARN_MB_COST / F_YARN_MB_FLAG is rewritten ONLY when its expression equals EXACTLY
-- the expected current text (else NOTICE + no-op). Products without an override (0/empty) compute identically.
-- All-or-nothing: if 0 < (matches of the exact old text) < 3 the migration RAISEs (partial drift = RP vs MB cost inconsistent);
-- 0 matches with all three already new = no-op; 0 otherwise = NOTICE + skip.
-- Backup bak_000618_formula holds old text/audit cols + whether THIS migration inserted the formula_param edge.
BEGIN;

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, display_group, display_order, created_at, created_by
)
VALUES ('MB_LDR_OVERRIDE', 'LDR Override (Manual)', 'LDR Override (Manual)', 'NUMBER', 'INPUT', NULL, TRUE,
        'Masterbatch & SP', 74, NOW(), 'migration_000618')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS bak_000618_formula (
    formula_code    VARCHAR(100) PRIMARY KEY,
    old_expression  TEXT NOT NULL,
    old_updated_at  TIMESTAMPTZ,
    old_updated_by  VARCHAR(100),
    edge_inserted   BOOLEAN NOT NULL DEFAULT FALSE,
    backed_up_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE bak_000618_formula IS 'Backup written by migration 000618; read by its down migration.';

CREATE TEMP TABLE _f618 (fcode TEXT, old_expr TEXT, new_expr TEXT) ON COMMIT DROP;
INSERT INTO _f618 VALUES
        ('F_YARN_RP_DOZING',
         'RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0)',
         'RP_DOZING_ACTUAL > 0 ? RP_DOZING_ACTUAL : (MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : (MB_SP_DOZING > 0 ? MB_SP_DOZING : 0))'),
        ('F_YARN_MB_COST',
         'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0))',
         'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (RP_DOZING_ACTUAL > 0 ? (MB_RATE_MKT * RP_DOZING_ACTUAL / 100.0) : (((1 + WASTE_PERC) * (MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING)) * MB_RATE_MKT / 100.0))'),
        ('F_YARN_MB_FLAG',
         'MB_SP_DOZING > 0 ? ''Y'' : ''N''',
         '(MB_LDR_OVERRIDE > 0 ? MB_LDR_OVERRIDE : MB_SP_DOZING) > 0 ? ''Y'' : ''N''')
    ;

DO $$
DECLARE
    r       RECORD;
    v_n     INTEGER;
    v_old   INTEGER;
    v_new   INTEGER;
BEGIN
    SELECT COUNT(*) FILTER (WHERE f.expression = t.old_expr), COUNT(*) FILTER (WHERE f.expression = t.new_expr)
      INTO v_old, v_new
      FROM _f618 t JOIN mst_formula f ON f.formula_code = t.fcode AND f.deleted_at IS NULL;
    IF v_old > 0 AND v_old < 3 THEN
        RAISE EXCEPTION '000618: partial formula drift (% of 3 expressions match the expected old text); aborting so RP and MB cost stay consistent', v_old;
    END IF;
    IF v_old = 0 THEN
        IF v_new = 3 THEN
            RAISE NOTICE '000618: formulas already migrated; no-op.';
        ELSE
            RAISE NOTICE '000618: formula expressions drifted from the expected text; formulas not rewritten.';
        END IF;
        RETURN;
    END IF;

    FOR r IN SELECT * FROM _f618
    LOOP
        INSERT INTO bak_000618_formula (formula_code, old_expression, old_updated_at, old_updated_by, edge_inserted)
        SELECT f.formula_code, f.expression, f.updated_at, f.updated_by,
               NOT EXISTS (SELECT 1 FROM formula_param fp
                           JOIN mst_parameter p ON p.id = fp.param_id AND p.param_code = 'MB_LDR_OVERRIDE' AND p.deleted_at IS NULL
                           WHERE fp.formula_id = f.id)
        FROM mst_formula f
        WHERE f.formula_code = r.fcode AND f.deleted_at IS NULL AND f.expression = r.old_expr
        ON CONFLICT (formula_code) DO NOTHING;

        UPDATE mst_formula
           SET expression = r.new_expr, updated_at = NOW(), updated_by = 'migration_000618'
         WHERE formula_code = r.fcode AND deleted_at IS NULL AND expression = r.old_expr;
        GET DIAGNOSTICS v_n = ROW_COUNT;

        IF v_n = 0 THEN
            RAISE NOTICE '000618: % not rewritten (0 rows): expression drifted from the expected text, or already migrated.', r.fcode;
        ELSE
            INSERT INTO formula_param (formula_id, param_id, sort_order)
            SELECT f.id, p.id,
                   COALESCE((SELECT MAX(fp2.sort_order) FROM formula_param fp2 WHERE fp2.formula_id = f.id), 0) + 1
            FROM mst_formula f
            JOIN mst_parameter p ON p.param_code = 'MB_LDR_OVERRIDE' AND p.deleted_at IS NULL
            WHERE f.formula_code = r.fcode AND f.deleted_at IS NULL
            ON CONFLICT DO NOTHING;
        END IF;
    END LOOP;
END $$;

COMMIT;
