-- 000523 down — restore OIL_NAME / OIL_RATE / OIL_GAIN from bak_oil_params_000523
-- and soft-delete OIL_GAIN_POY_DEFAULT (only the row this migration created).
--
-- Reverse order guarantees 000524's down already soft-deleted
-- F_YARN_OIL_GAIN_POY_DEFAULT and removed the OIL_GAIN_POY_DEFAULT edges, and
-- 000525's down already removed the CAPP rows it seeded.

BEGIN;

UPDATE mst_parameter p
SET param_category         = b.param_category,
    lookup_master_code     = b.lookup_master_code,
    lookup_fill_group_code = b.lookup_fill_group_code,
    lookup_source_column   = b.lookup_source_column,
    uom_id                 = b.uom_id,
    notes                  = b.notes,
    updated_at             = b.updated_at,
    updated_by             = b.updated_by
FROM bak_oil_params_000523 b
WHERE p.param_code = b.param_code
  AND p.deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(),
    deleted_by = 'migration_000523_down'
WHERE param_code = 'OIL_GAIN_POY_DEFAULT'
  AND created_by = 'seed_000523'
  AND deleted_at IS NULL;

DROP TABLE IF EXISTS bak_oil_params_000523;

COMMIT;
