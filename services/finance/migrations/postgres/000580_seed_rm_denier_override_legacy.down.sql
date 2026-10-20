-- 000580 down — undo the legacy RM denier seed.
--
-- 1. Delete the override rows created by the seed (cpdo_created_by =
--    'migration_000580'); manual overrides are untouched.
-- 2. Restore SPANDEX_DRAFT from bak_000580_spandex_draft, only where the
--    current value is still the seeded value (later manual edits are kept).
-- 3. Restore cpm_rm_ratio_mode from bak_000580_rm_ratio_mode, only where the
--    current mode is still the seeded 'DENIER'.
-- 4. Drop the migration-owned backup tables.
--
-- Idempotent: every step is guarded; a second run is a no-op.

BEGIN;

SET LOCAL lock_timeout = '10s';

DELETE FROM cost_product_rm_denier_override
WHERE cpdo_created_by = 'migration_000580';

DO $$
BEGIN
    IF to_regclass('bak_000580_spandex_draft') IS NOT NULL THEN
        UPDATE cost_product_parameter c
        SET cpp_value_numeric = b.old_value,
            cpp_updated_at    = NOW(),
            cpp_updated_by    = 'migration_000580_down'
        FROM bak_000580_spandex_draft b
        JOIN cost_product_master m ON m.cpm_product_code = b.product_code
        JOIN mst_parameter p       ON p.param_code = 'SPANDEX_DRAFT' AND p.deleted_at IS NULL
        WHERE c.cpp_product_sys_id = m.cpm_product_sys_id
          AND c.cpp_param_id       = p.id
          AND c.cpp_value_numeric  = b.new_value;
    END IF;

    IF to_regclass('bak_000580_rm_ratio_mode') IS NOT NULL THEN
        UPDATE cost_product_master m
        SET cpm_rm_ratio_mode = b.old_mode,
            cpm_updated_at    = NOW(),
            cpm_updated_by    = 'migration_000580_down'
        FROM bak_000580_rm_ratio_mode b
        WHERE m.cpm_product_code  = b.product_code
          AND m.cpm_rm_ratio_mode = b.new_mode;
    END IF;
END $$;

DROP TABLE IF EXISTS bak_000580_spandex_draft;
DROP TABLE IF EXISTS bak_000580_rm_ratio_mode;

COMMIT;
