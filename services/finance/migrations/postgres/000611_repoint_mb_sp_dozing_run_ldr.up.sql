BEGIN;

-- D31 L1: MB_SP_DOZING reads the actual LDR (mbs_run_ldr_pct) instead of the
-- retired, contaminated mbs_dozing column (D30). mbs_run_ldr_pct is registered
-- in mst_lookup_master_column since 000414. Guarded: only rows still pointing at
-- mbs_dozing are updated, so re-running is a no-op. Stored values are untouched;
-- the next lookup sync reads the new column.
UPDATE mst_parameter
   SET lookup_source_column = 'mbs_run_ldr_pct',
       updated_at = NOW(),
       updated_by = 'migration_000611'
 WHERE param_code = 'MB_SP_DOZING'
   AND deleted_at IS NULL
   AND lookup_source_column = 'mbs_dozing';

COMMIT;
