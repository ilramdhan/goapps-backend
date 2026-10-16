-- 000576 down — restore OIL_NAME values from bak_oil_name_000576 and remove
-- the oil-group mappings 000576 inserted (ids in bak_oil_group_map_000576).
--
-- Only rows still carrying the 'migration_000576' marker are reverted, so a
-- value a user changed after the migration is not overwritten with stale text.
-- Mappings are removed only when listed in the backup AND still marked
-- cptog_created_by = 'migration_000576'. Then drop both backup tables.

BEGIN;

SET LOCAL lock_timeout = '10s';

DO $$
BEGIN
    IF to_regclass('public.bak_oil_name_000576') IS NULL THEN
        RAISE NOTICE '000576 down: OIL_NAME backup table missing — nothing to restore.';
    ELSE
        UPDATE cost_product_parameter cpp
        SET cpp_value_text    = b.old_value_text,
            cpp_value_numeric = b.old_value_numeric,
            cpp_value_flag    = b.old_value_flag,
            cpp_filled_at     = b.old_filled_at,
            cpp_filled_by     = b.old_filled_by,
            cpp_updated_at    = b.old_updated_at,
            cpp_updated_by    = b.old_updated_by
        FROM bak_oil_name_000576 b
        WHERE b.had_row
          AND cpp.cpp_product_sys_id = b.product_sys_id
          AND cpp.cpp_param_id = b.param_id
          AND cpp.cpp_filled_by = 'migration_000576';
    END IF;

    IF to_regclass('public.bak_oil_group_map_000576') IS NULL THEN
        RAISE NOTICE '000576 down: mapping backup table missing — no mapping removed.';
    ELSE
        DELETE FROM cost_product_type_oil_group og
        USING bak_oil_group_map_000576 b
        WHERE og.cptog_id = b.cptog_id
          AND og.cptog_created_by = 'migration_000576';
    END IF;
END $$;

DROP TABLE IF EXISTS bak_oil_name_000576;
DROP TABLE IF EXISTS bak_oil_group_map_000576;

COMMIT;
