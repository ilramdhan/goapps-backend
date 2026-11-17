-- 000610: Unlink MC_SPEED / MC_EFFICIENCY from the MC_NAME (machine) fill group.
--
-- Speed and efficiency are now per-product values (cost_product_parameter),
-- not values auto-filled from mst_machine. This migration only changes the
-- configuration that makes them children of MC_NAME:
--   - mst_parameter.lookup_fill_group_code / lookup_source_column cleared for
--     MC_SPEED and MC_EFFICIENCY (they stop being auto-added / auto-overwritten).
--   - mst_lookup_master_column rows mc_speed / mc_efficiency for MACHINE removed
--     so the Parameter form no longer offers them as Source Column.
--
-- Product values are NOT touched: cost_product_parameter,
-- cost_product_applicable_param, mst_machine (mc_speed / mc_efficiency columns
-- and their data), and snapshots / cost results stay exactly as they are.
-- Down migration 000610 restores the links and registry rows.

BEGIN;

UPDATE mst_parameter
SET lookup_fill_group_code = NULL,
    lookup_source_column   = NULL,
    updated_at             = NOW(),
    updated_by             = 'migration_000610'
WHERE param_code IN ('MC_SPEED', 'MC_EFFICIENCY')
  AND deleted_at IS NULL;

DELETE FROM mst_lookup_master_column
WHERE lmc_master_code = 'MACHINE'
  AND lmc_column_name IN ('mc_speed', 'mc_efficiency');

COMMIT;
