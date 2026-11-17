-- 000610 down: restore the MC_NAME fill-group links for MC_SPEED / MC_EFFICIENCY
-- and re-register the MACHINE registry rows (values as seeded by 000394 / 000395).
-- Idempotent: registry rows are inserted only when missing; links are set by code.
-- Product values and mst_machine data are never touched.

BEGIN;

UPDATE mst_parameter
SET lookup_fill_group_code = 'MC_NAME',
    lookup_source_column   = 'mc_speed',
    updated_at             = NOW(),
    updated_by             = 'migration_000610_down'
WHERE param_code = 'MC_SPEED'
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET lookup_fill_group_code = 'MC_NAME',
    lookup_source_column   = 'mc_efficiency',
    updated_at             = NOW(),
    updated_by             = 'migration_000610_down'
WHERE param_code = 'MC_EFFICIENCY'
  AND deleted_at IS NULL;

INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
SELECT 'MACHINE', 'mc_speed', 'Machine Speed (m/min)', 'NUMBER', 10
WHERE NOT EXISTS (
    SELECT 1 FROM mst_lookup_master_column
    WHERE lmc_master_code = 'MACHINE' AND lmc_column_name = 'mc_speed'
);

INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
SELECT 'MACHINE', 'mc_efficiency', 'Machine Efficiency (%)', 'NUMBER', 20
WHERE NOT EXISTS (
    SELECT 1 FROM mst_lookup_master_column
    WHERE lmc_master_code = 'MACHINE' AND lmc_column_name = 'mc_efficiency'
);

COMMIT;
