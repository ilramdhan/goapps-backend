-- 000535: grade weights AE_WT / A9_WT / A_WT / B_WT / C_WT -> CALCULATED.
--
-- Business rule (confirmed 2026-09-30): the five grade weights are produced by
-- F_YARN_<G>_WT (tx_weight(), 000530). They are not manual inputs, so the
-- Product Master must show them as calculated/read-only. AX_WT stays INPUT.
--
-- Root cause: 000381 seeded them as CALCULATED, but 000406 wiped
-- mst_parameter and 000407 re-seeded them as 'INPUT' (lines 24-35).
-- NET_BOB_WT was re-seeded as CALCULATED (000407:97) and is not touched here.
--
-- The UI (parameters-tab.tsx) renders read-only only when
-- param_category = 'CALCULATED', so this data change alone fixes the UI.
--
-- Stored cost_product_parameter values for these params are NOT deleted.
-- They stay in place and the engine overwrites them with the formula result.
--
-- Guarded: only rows still at param_category = 'INPUT' change. The prior
-- values go to bak_grade_wt_params_000535 (ON CONFLICT DO NOTHING, so the first
-- run's values win) and the down migration restores them exactly.
-- Marker updated_by = 'grade_wt_calc_000535'.

BEGIN;

CREATE TABLE IF NOT EXISTS bak_grade_wt_params_000535 (
    param_code     VARCHAR(50)  PRIMARY KEY,
    param_category VARCHAR(20)  NOT NULL,
    updated_at     TIMESTAMPTZ,
    updated_by     VARCHAR(200),
    backed_up_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE bak_grade_wt_params_000535 IS
    'Backup written by migration 000535 (grade weights CALCULATED); read by its down migration. Safe to drop once 000535 is final.';

INSERT INTO bak_grade_wt_params_000535 (param_code, param_category, updated_at, updated_by)
SELECT p.param_code, p.param_category, p.updated_at, p.updated_by
FROM mst_parameter p
WHERE p.param_code IN ('AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT')
  AND p.param_category = 'INPUT'
  AND p.deleted_at IS NULL
ON CONFLICT (param_code) DO NOTHING;

UPDATE mst_parameter
SET param_category = 'CALCULATED',
    updated_at     = NOW(),
    updated_by     = 'grade_wt_calc_000535'
WHERE param_code IN ('AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT')
  AND param_category = 'INPUT'
  AND deleted_at IS NULL;

DO $$
DECLARE
    v_changed    INT;
    v_calculated INT;
    v_ax_input   INT;
BEGIN
    SELECT COUNT(*) INTO v_changed
    FROM mst_parameter
    WHERE param_code IN ('AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT')
      AND deleted_at IS NULL
      AND updated_by = 'grade_wt_calc_000535';

    SELECT COUNT(*) INTO v_calculated
    FROM mst_parameter
    WHERE param_code IN ('AE_WT', 'A9_WT', 'A_WT', 'B_WT', 'C_WT')
      AND deleted_at IS NULL
      AND param_category = 'CALCULATED';

    SELECT COUNT(*) INTO v_ax_input
    FROM mst_parameter
    WHERE param_code = 'AX_WT'
      AND deleted_at IS NULL
      AND param_category = 'INPUT';

    RAISE NOTICE '000535: % grade-weight params changed INPUT -> CALCULATED; % of 5 are now CALCULATED', v_changed, v_calculated;
    IF v_ax_input <> 1 THEN
        RAISE NOTICE '000535: WARNING AX_WT is not a single active INPUT param (found %); it was not touched', v_ax_input;
    END IF;
END $$;

COMMIT;
