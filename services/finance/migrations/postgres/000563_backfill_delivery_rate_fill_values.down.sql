-- 000563 down — restore DELIVERY_BOB_RATE / DELIVERY_BOX_RATE values from the backup table.
BEGIN;

DO $$
BEGIN
    IF to_regclass('bak_000563_delivery_rate_values') IS NULL THEN
        RAISE NOTICE '000563 down: backup table missing, nothing to restore';
        RETURN;
    END IF;

    -- Restore rows that existed (only if still carrying this migration's marker).
    UPDATE cost_product_parameter cpp
       SET cpp_value_numeric = b.old_value_numeric,
           cpp_value_text    = b.old_value_text,
           cpp_value_flag    = b.old_value_flag,
           cpp_filled_at     = COALESCE(b.old_filled_at, cpp.cpp_filled_at),
           cpp_filled_by     = COALESCE(b.old_filled_by, cpp.cpp_filled_by),
           cpp_updated_at    = b.old_updated_at,
           cpp_updated_by    = b.old_updated_by
      FROM bak_000563_delivery_rate_values b
     WHERE b.had_row
       AND cpp.cpp_product_sys_id = b.product_sys_id AND cpp.cpp_param_id = b.param_id
       AND cpp.cpp_updated_by = 'backfill_delivery_rate_000563';

    -- Remove inserted rows (only if untouched since).
    DELETE FROM cost_product_parameter cpp
     USING bak_000563_delivery_rate_values b
     WHERE NOT b.had_row
       AND cpp.cpp_product_sys_id = b.product_sys_id AND cpp.cpp_param_id = b.param_id
       AND cpp.cpp_created_by = 'backfill_delivery_rate_000563'
       AND cpp.cpp_updated_at IS NULL;
END $$;

DROP TABLE IF EXISTS bak_000563_delivery_rate_values;

COMMIT;
