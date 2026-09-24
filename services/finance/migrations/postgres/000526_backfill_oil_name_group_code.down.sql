-- 000526 down — restore OIL_NAME values from bak_oil_name_000526.
--
-- Only rows still carrying the 'migration_000526' marker are reverted, so a
-- value a user changed after the backfill is not overwritten with stale text.
--   had_row = TRUE  -> restore the original value + filled_* / updated_* columns
--   had_row = FALSE -> delete the row the migration inserted
-- Then drop the backup table.

BEGIN;

DO $$
BEGIN
    IF to_regclass('public.bak_oil_name_000526') IS NULL THEN
        RAISE NOTICE '000526 down: backup table missing — nothing to restore.';
        RETURN;
    END IF;

    UPDATE cost_product_parameter cpp
    SET cpp_value_text    = b.old_value_text,
        cpp_value_numeric = b.old_value_numeric,
        cpp_value_flag    = b.old_value_flag,
        cpp_filled_at     = b.old_filled_at,
        cpp_filled_by     = b.old_filled_by,
        cpp_updated_at    = b.old_updated_at,
        cpp_updated_by    = b.old_updated_by
    FROM bak_oil_name_000526 b
    WHERE b.had_row
      AND cpp.cpp_product_sys_id = b.product_sys_id
      AND cpp.cpp_param_id = b.param_id
      AND cpp.cpp_filled_by = 'migration_000526';

    DELETE FROM cost_product_parameter cpp
    USING bak_oil_name_000526 b
    WHERE NOT b.had_row
      AND cpp.cpp_product_sys_id = b.product_sys_id
      AND cpp.cpp_param_id = b.param_id
      AND cpp.cpp_created_by = 'migration_000526'
      AND cpp.cpp_filled_by = 'migration_000526';
END $$;

DROP TABLE IF EXISTS bak_oil_name_000526;

COMMIT;
