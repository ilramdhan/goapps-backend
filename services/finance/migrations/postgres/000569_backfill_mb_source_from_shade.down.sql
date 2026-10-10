-- 000569 down — restore values and remove CAPP rows written by the shade-driven MB source backfill.
BEGIN;

DO $$
BEGIN
    IF to_regclass('bak_000569_values') IS NULL THEN
        RAISE NOTICE '000569 down: backup table missing, nothing to restore';
        RETURN;
    END IF;

    -- Restore pre-existing (empty) rows still carrying this migration's marker.
    UPDATE cost_product_parameter cpp
       SET cpp_value_numeric = b.old_value_numeric, cpp_value_text = b.old_value_text,
           cpp_value_flag = b.old_value_flag, cpp_value_mb_spin_id = b.old_mb_spin_id,
           cpp_filled_at = COALESCE(b.old_filled_at, cpp.cpp_filled_at),
           cpp_filled_by = COALESCE(b.old_filled_by, cpp.cpp_filled_by),
           cpp_updated_at = b.old_updated_at, cpp_updated_by = b.old_updated_by
      FROM bak_000569_values b
     WHERE b.had_row AND cpp.cpp_product_sys_id = b.product_sys_id AND cpp.cpp_param_id = b.param_id
       AND cpp.cpp_updated_by = 'backfill_mb_source_000569';

    -- Remove rows inserted by the migration (only if untouched since).
    DELETE FROM cost_product_parameter cpp
     USING bak_000569_values b
     WHERE NOT b.had_row AND cpp.cpp_product_sys_id = b.product_sys_id AND cpp.cpp_param_id = b.param_id
       AND cpp.cpp_created_by = 'backfill_mb_source_000569' AND cpp.cpp_updated_at IS NULL;
END $$;

DO $$
BEGIN
    IF to_regclass('bak_000569_capp') IS NOT NULL THEN
        -- CAPP rows inserted here, only while no value row remains for them.
        DELETE FROM cost_product_applicable_param c
         USING bak_000569_capp b
         WHERE c.capp_product_sys_id = b.product_sys_id AND c.capp_param_id = b.param_id
           AND c.capp_created_by = 'backfill_mb_source_000569'
           AND NOT EXISTS (SELECT 1 FROM cost_product_parameter v
                           WHERE v.cpp_product_sys_id = c.capp_product_sys_id AND v.cpp_param_id = c.capp_param_id);
    END IF;
END $$;

DROP TABLE IF EXISTS bak_000569_values;
DROP TABLE IF EXISTS bak_000569_capp;

COMMIT;
