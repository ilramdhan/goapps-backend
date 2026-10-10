-- 000568 — F_YARN_MB_COST (result MB_COST_MKT): apply the SUPERBA branch of 000567 to the
-- formula text that actually lives in PROD.
--
-- Why: in PROD an admin edited F_YARN_MB_COST on the web (2026-09-24) to
--     ((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0
-- so the exact-text guard of 000567 matched 0 rows there (NOTICE only) and SUPERBA products
-- kept the old arm. Target everywhere (prod text kept verbatim in the else arm):
--     IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)
--
-- Case A (prod)       : expression = the web-edited text above.
-- Case B (lower envs) : expression = 000567's output AND updated_by = 'superba_mb_cost_000567'
--                       (also syncs them to prod's waste-adjusted arm, same rationale as 000560).
-- Both require the result param MB_COST_MKT. Anything else (further hand edits) is left alone;
-- a NOTICE (not an exception) reports 0 rows so drift is visible without failing.
--
-- Backup: bak_000568_formula (pattern of 000564) holds the exact old expression/audit columns and
-- whether THIS migration inserted the F_YARN_MB_COST -> WASTE_PERC edge (edge_inserted), for an exact down.
-- WASTE_PERC is an INPUT param (000407) with no formula, so the edge cannot create a cycle.
-- IS_SUPERBA / SUPERBA_MB_COST remain engine keys (no mst_parameter row, no edge).
BEGIN;

CREATE TABLE IF NOT EXISTS bak_000568_formula (
    formula_code    VARCHAR(100) PRIMARY KEY,
    old_expression  TEXT NOT NULL,
    old_updated_at  TIMESTAMPTZ,
    old_updated_by  VARCHAR(100),
    edge_inserted   BOOLEAN NOT NULL DEFAULT FALSE,
    backed_up_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE bak_000568_formula IS 'Backup written by migration 000568; read by its down migration.';

DO $$
DECLARE
    v_updated INTEGER;
    v_target  CONSTANT TEXT := 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0)';
BEGIN
    -- backup (only formulas about to be rewritten)
    INSERT INTO bak_000568_formula (formula_code, old_expression, old_updated_at, old_updated_by, edge_inserted)
    SELECT f.formula_code, f.expression, f.updated_at, f.updated_by,
           NOT EXISTS (SELECT 1 FROM formula_param fp
                       JOIN mst_parameter wp ON wp.id = fp.param_id AND wp.param_code = 'WASTE_PERC' AND wp.deleted_at IS NULL
                       WHERE fp.formula_id = f.id)
    FROM mst_formula f
    WHERE f.formula_code = 'F_YARN_MB_COST'
      AND f.deleted_at IS NULL
      AND (f.expression = '((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0'
        OR (f.expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)'
            AND f.updated_by = 'superba_mb_cost_000567'))
      AND EXISTS (SELECT 1 FROM mst_parameter p
                  WHERE p.id = f.result_param_id AND p.param_code = 'MB_COST_MKT' AND p.deleted_at IS NULL)
    ON CONFLICT (formula_code) DO NOTHING;

    UPDATE mst_formula f
    SET expression = v_target,
        updated_at = NOW(),
        updated_by = 'superba_mb_cost_000568'
    WHERE f.formula_code = 'F_YARN_MB_COST'
      AND f.deleted_at IS NULL
      AND (f.expression = '((1 + WASTE_PERC) * MB_SP_DOZING) * MB_RATE_MKT / 100.0'
        OR (f.expression = 'IS_SUPERBA == 1 ? SUPERBA_MB_COST : (MB_RATE_MKT * MB_SP_DOZING / 100.0)'
            AND f.updated_by = 'superba_mb_cost_000567'))
      AND EXISTS (SELECT 1 FROM mst_parameter p
                  WHERE p.id = f.result_param_id AND p.param_code = 'MB_COST_MKT' AND p.deleted_at IS NULL);
    GET DIAGNOSTICS v_updated = ROW_COUNT;

    IF v_updated = 0 THEN
        RAISE NOTICE '000568: F_YARN_MB_COST not rewritten (0 rows): expression drifted from both known texts, or already migrated. Inspect mst_formula manually.';
    ELSE
        RAISE NOTICE '000568: F_YARN_MB_COST rewritten (% row)', v_updated;
    END IF;
END $$;

-- edge F_YARN_MB_COST -> WASTE_PERC, only for the formula this migration rewrote
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id,
       COALESCE((SELECT MAX(fp2.sort_order) FROM formula_param fp2 WHERE fp2.formula_id = f.id), 0) + 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'WASTE_PERC' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_MB_COST' AND f.deleted_at IS NULL
  AND f.updated_by = 'superba_mb_cost_000568'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

COMMIT;
