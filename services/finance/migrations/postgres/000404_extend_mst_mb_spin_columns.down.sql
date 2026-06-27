DELETE FROM mst_lookup_master_column
WHERE lmc_master_code = 'MB_SPIN'
  AND lmc_column_name IN ('mbs_cc', 'mbs_cost_rate_mkt');

ALTER TABLE mst_mb_spin
    DROP COLUMN IF EXISTS mbs_cc,
    DROP COLUMN IF EXISTS mbs_cost_rate_mkt;
