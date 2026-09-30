-- Reverse 000546 (loss-param dedup + V-loss formulas).
--
-- Order:
--   1. Reactivate the 4 legacy params 000546 deactivated (marker-keyed).
--   2. Restore the 000408 edges and expressions of the three V-loss formulas
--      (only formulas still carrying the 000546 marker and the 000546 text).
--   3. Restore every CPP value 000546 overwrote from bak_loss_param_000546
--      (grade codes -> original grade names, previous numeric children).
--   4. Delete the CPP / CAPP rows 000546 inserted (created_by marker).
--   5. Drop the backup table.
--
-- CPP rows edited by a user after 000546 (cpp_updated_by no longer the marker)
-- are left as the user set them, together with their CAPP row. Idempotent.

BEGIN;

DO $$
DECLARE
    v_react    INTEGER;
    v_edges_d  INTEGER;
    v_edges_i  INTEGER;
    v_formulas INTEGER;
    v_restored BIGINT := 0;
    v_cpp_del  BIGINT;
    v_capp_del BIGINT;
BEGIN
    -- 1. Reactivate
    UPDATE mst_parameter
    SET is_active = TRUE, updated_at = NULL, updated_by = NULL
    WHERE param_code IN ('STD_VALUE_LOSS', 'VALUE_LOSS', 'NON_STD_SPECIAL_PROD', 'BC_SPECIAL_PROD')
      AND deleted_at IS NULL
      AND is_active = FALSE
      AND updated_by = 'loss_param_dedup_000546';
    GET DIAGNOSTICS v_react = ROW_COUNT;

    -- 2a. Edges back to 000408:166-171 (while the marker still identifies them)
    DELETE FROM formula_param fp
    USING mst_formula f
    WHERE fp.formula_id = f.id
      AND f.formula_code IN ('F_YARN_NON_STD_LOSS', 'F_YARN_BC_LOSS_CAP', 'F_YARN_BC_LOSS_DEL')
      AND f.deleted_at IS NULL
      AND f.updated_by = 'loss_param_dedup_000546';
    GET DIAGNOSTICS v_edges_d = ROW_COUNT;

    INSERT INTO formula_param (formula_id, param_id, sort_order)
    SELECT f.id, p.id, e.sort_order
    FROM (VALUES
        ('F_YARN_BC_LOSS_CAP',  'CAPTIVE_COST_BEFORE_QLOSS',  1),
        ('F_YARN_BC_LOSS_CAP',  'BC_SPECIAL_PROD',            2),
        ('F_YARN_BC_LOSS_CAP',  'VALUE_LOSS',                 3),
        ('F_YARN_BC_LOSS_DEL',  'DELIVERY_COST_BEFORE_QLOSS', 1),
        ('F_YARN_BC_LOSS_DEL',  'BC_SPECIAL_PROD',            2),
        ('F_YARN_BC_LOSS_DEL',  'VALUE_LOSS',                 3),
        ('F_YARN_NON_STD_LOSS', 'CAPTIVE_COST_BEFORE_QLOSS',  1),
        ('F_YARN_NON_STD_LOSS', 'NON_STD_SPECIAL_PROD',       2),
        ('F_YARN_NON_STD_LOSS', 'VALUE_LOSS',                 3)
    ) AS e(formula_code, param_code, sort_order)
    JOIN mst_formula f ON f.formula_code = e.formula_code
                      AND f.deleted_at IS NULL
                      AND f.updated_by = 'loss_param_dedup_000546'
    JOIN mst_parameter p ON p.param_code = e.param_code AND p.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id
    );
    GET DIAGNOSTICS v_edges_i = ROW_COUNT;

    -- 2b. Expressions back to 000408:43-45. updated_at / updated_by -> NULL,
    -- the state 000408 left them in.
    UPDATE mst_formula f
    SET expression = CASE f.formula_code
            WHEN 'F_YARN_NON_STD_LOSS' THEN 'CAPTIVE_COST_BEFORE_QLOSS * (NON_STD_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)'
            WHEN 'F_YARN_BC_LOSS_CAP'  THEN 'CAPTIVE_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)'
            WHEN 'F_YARN_BC_LOSS_DEL'  THEN 'DELIVERY_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)'
        END,
        updated_at = NULL,
        updated_by = NULL
    WHERE f.deleted_at IS NULL
      AND f.updated_by = 'loss_param_dedup_000546'
      AND (
          (f.formula_code = 'F_YARN_NON_STD_LOSS'
           AND f.expression = '(AE_PERC + A9_PERC + A_PERC) / 100.0 * NS_LOSS')
       OR (f.formula_code = 'F_YARN_BC_LOSS_CAP'
           AND f.expression = '(CAPTIVE_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0')
       OR (f.formula_code = 'F_YARN_BC_LOSS_DEL'
           AND f.expression = '(DELIVERY_COST_BEFORE_QLOSS - STD_SP_BC) * (B_PERC + C_PERC) / 100.0')
      );
    GET DIAGNOSTICS v_formulas = ROW_COUNT;

    -- 3. Restore overwritten CPP values (only rows still stamped by 000546).
    IF to_regclass('public.bak_loss_param_000546') IS NOT NULL THEN
        UPDATE cost_product_parameter c
        SET cpp_value_text    = b.old_value_text,
            cpp_value_numeric = b.old_value_numeric,
            cpp_value_flag    = b.old_value_flag,
            cpp_updated_at    = b.old_updated_at,
            cpp_updated_by    = b.old_updated_by
        FROM bak_loss_param_000546 b
        WHERE c.cpp_product_sys_id = b.cpp_product_sys_id
          AND c.cpp_param_id = b.cpp_param_id
          AND c.cpp_updated_by = 'loss_param_dedup_000546';
        GET DIAGNOSTICS v_restored = ROW_COUNT;
    END IF;

    -- 4. Delete rows 000546 inserted (untouched since).
    DELETE FROM cost_product_parameter
    WHERE cpp_created_by = 'loss_param_dedup_000546'
      AND (cpp_updated_by IS NULL OR cpp_updated_by = 'loss_param_dedup_000546');
    GET DIAGNOSTICS v_cpp_del = ROW_COUNT;

    -- CAPP rows 000546 inserted go only where no CPP row remains for the same
    -- (product, param): a user-edited CPP row kept above stays visible
    -- (LoadCAPP / LoadCAPPText inner-join CAPP).
    DELETE FROM cost_product_applicable_param a
    WHERE a.capp_created_by = 'loss_param_dedup_000546'
      AND NOT EXISTS (
          SELECT 1 FROM cost_product_parameter c
          WHERE c.cpp_product_sys_id = a.capp_product_sys_id
            AND c.cpp_param_id = a.capp_param_id
      );
    GET DIAGNOSTICS v_capp_del = ROW_COUNT;

    RAISE NOTICE '000546 down: params reactivated=%, edges deleted=% / restored=%, formulas reverted=%, CPP restored=%, CPP deleted=%, CAPP deleted=%',
        v_react, v_edges_d, v_edges_i, v_formulas, v_restored, v_cpp_del, v_capp_del;
END $$;

DROP TABLE IF EXISTS bak_loss_param_000546;

COMMIT;
