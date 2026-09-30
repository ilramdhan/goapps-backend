-- 000534: AX_WT is a pure manual input -> deactivate F_YARN_AX_WT_FROM_MKT.
--
-- Business rule (confirmed 2026-09-30): AX_WT is entered by hand in the
-- Product Master (cost_product_parameter). Every calc type (ACTUAL, FORECAST,
-- SELLING and the MB batch) must use that input as-is. It must NOT be copied
-- from the marketing_result / SELLING snapshot.
--
-- Root cause: 000408 seeded the active FROM_MARKETING formula
--   F_YARN_AX_WT_FROM_MKT = marketing_result(product,'AX_WT',period) -> AX_WT
-- The loader picks up every active formula whose result param is in the
-- product's CAPP, so every product with AX_WT evaluated it, and
-- marketing_result prefers the SELLING snapshot. A stale snapshot therefore
-- overwrote the manual AX_WT and, through F_YARN_<G>_WT, every grade weight.
--
-- The formula is DEACTIVATED (is_active = FALSE), not soft-deleted: it stays
-- visible in Master Formula and keeps the result_param slot. The engine also
-- ignores the snapshot for AX_WT (costcalc.manualInputOnlyParams), so a
-- reactivated formula still resolves to the CAPP value.
--
-- The other FROM_MARKETING formulas (CAPTIVE_NO_OF_BOB, DELIVERY_NO_OF_BOB,
-- HEATSET_CODE, DOZING_ADJUST) are left unchanged.
--
-- Guarded: only fires on the exact 000408 row (active, same type and
-- expression). The prior is_active/updated_* values go to
-- bak_ax_wt_formula_000534 so the down migration restores them exactly.
-- Marker updated_by = 'ax_wt_input_000534'.

BEGIN;

CREATE TABLE IF NOT EXISTS bak_ax_wt_formula_000534 (
    formula_code VARCHAR(50)  PRIMARY KEY,
    is_active    BOOLEAN      NOT NULL,
    updated_at   TIMESTAMPTZ,
    updated_by   VARCHAR(200),
    backed_up_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE bak_ax_wt_formula_000534 IS
    'Backup written by migration 000534 (AX_WT manual input); read by its down migration. Safe to drop once 000534 is final.';

INSERT INTO bak_ax_wt_formula_000534 (formula_code, is_active, updated_at, updated_by)
SELECT f.formula_code, f.is_active, f.updated_at, f.updated_by
FROM mst_formula f
WHERE f.formula_code = 'F_YARN_AX_WT_FROM_MKT'
  AND f.deleted_at IS NULL
  AND f.is_active = TRUE
  AND f.formula_type = 'FROM_MARKETING'
  AND f.expression = 'marketing_result(product,''AX_WT'',period)'
ON CONFLICT (formula_code) DO NOTHING;

UPDATE mst_formula
SET is_active  = FALSE,
    updated_at = NOW(),
    updated_by = 'ax_wt_input_000534'
WHERE formula_code = 'F_YARN_AX_WT_FROM_MKT'
  AND deleted_at IS NULL
  AND is_active = TRUE
  AND formula_type = 'FROM_MARKETING'
  AND expression = 'marketing_result(product,''AX_WT'',period)';

DO $$
DECLARE
    v_deactivated INT;
    v_still_active INT;
BEGIN
    SELECT COUNT(*) INTO v_deactivated
    FROM mst_formula
    WHERE formula_code = 'F_YARN_AX_WT_FROM_MKT'
      AND deleted_at IS NULL
      AND updated_by = 'ax_wt_input_000534';

    SELECT COUNT(*) INTO v_still_active
    FROM mst_formula
    WHERE formula_code = 'F_YARN_AX_WT_FROM_MKT'
      AND deleted_at IS NULL
      AND is_active = TRUE;

    IF v_deactivated = 1 THEN
        RAISE NOTICE '000534: deactivated F_YARN_AX_WT_FROM_MKT; AX_WT is now a pure CAPP input';
    ELSIF v_still_active > 0 THEN
        RAISE NOTICE '000534: F_YARN_AX_WT_FROM_MKT is active but no longer matches the 000408 row; left unchanged (engine still ignores the snapshot for AX_WT)';
    ELSE
        RAISE NOTICE '000534: F_YARN_AX_WT_FROM_MKT already inactive or absent; nothing to do';
    END IF;
END $$;

COMMIT;
