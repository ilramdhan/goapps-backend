-- 000577 down — exact reverse of 000577: restore OIL_NAME rows from
-- bak_oil_name_000577, delete the CAPP rows and oil-group mappings 000577
-- added, restore ATY / ACY oil class, drop the backup tables.
--
-- Only OIL_NAME rows still carrying the 'migration_000577' filled_by marker
-- are reverted, so a later user edit is not overwritten.

BEGIN;

SET LOCAL lock_timeout = '10s';

DO $$
BEGIN
    IF to_regclass('public.bak_oil_name_000577') IS NULL THEN
        RAISE NOTICE '000577 down: OIL_NAME backup table missing — nothing to restore.';
    ELSE
        UPDATE cost_product_parameter cpp
        SET cpp_value_text    = b.old_value_text,
            cpp_value_numeric = b.old_value_numeric,
            cpp_value_flag    = b.old_value_flag,
            cpp_filled_at     = b.old_filled_at,
            cpp_filled_by     = b.old_filled_by,
            cpp_updated_at    = b.old_updated_at,
            cpp_updated_by    = b.old_updated_by
        FROM bak_oil_name_000577 b
        WHERE b.had_row
          AND cpp.cpp_product_sys_id = b.product_sys_id
          AND cpp.cpp_param_id = b.param_id
          AND cpp.cpp_filled_by = 'migration_000577';

        DELETE FROM cost_product_parameter cpp
        USING bak_oil_name_000577 b
        WHERE NOT b.had_row
          AND cpp.cpp_product_sys_id = b.product_sys_id
          AND cpp.cpp_param_id = b.param_id
          AND cpp.cpp_filled_by = 'migration_000577';
    END IF;

    -- CAPP rows added by 000577 (user values in cost_product_parameter are inert without them).
    DELETE FROM cost_product_applicable_param
    WHERE capp_created_by = 'migration_000577';

    IF to_regclass('public.bak_oil_group_map_000577') IS NULL THEN
        RAISE NOTICE '000577 down: mapping backup table missing — no mapping removed.';
    ELSE
        DELETE FROM cost_product_type_oil_group og
        USING bak_oil_group_map_000577 b
        WHERE og.cptog_id = b.cptog_id
          AND og.cptog_created_by = 'migration_000577';
    END IF;

    IF to_regclass('public.bak_oil_class_000577') IS NULL THEN
        RAISE NOTICE '000577 down: oil-class backup table missing — class not restored.';
    ELSE
        UPDATE cost_product_type t
        SET cpt_oil_class  = b.old_oil_class,
            cpt_updated_at = NOW()
        FROM bak_oil_class_000577 b
        WHERE t.cpt_type_code = b.type_code;
    END IF;
END $$;

DROP TABLE IF EXISTS bak_oil_name_000577;
DROP TABLE IF EXISTS bak_oil_group_map_000577;
DROP TABLE IF EXISTS bak_oil_class_000577;

COMMIT;
