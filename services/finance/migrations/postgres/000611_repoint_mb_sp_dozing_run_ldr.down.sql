BEGIN;

-- Restore the pre-000611 source column (seeded by 000407). Guarded by current value.
UPDATE mst_parameter
   SET lookup_source_column = 'mbs_dozing',
       updated_at = NOW(),
       updated_by = 'migration_000611_down'
 WHERE param_code = 'MB_SP_DOZING'
   AND deleted_at IS NULL
   AND lookup_source_column = 'mbs_run_ldr_pct';

COMMIT;
