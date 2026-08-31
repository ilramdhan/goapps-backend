-- Down 000499 — restore the original display name for MB_SPIN.mbs_cc,
-- as originally seeded in 000404_extend_mst_mb_spin_columns.up.sql.

BEGIN;

UPDATE mst_lookup_master_column
SET lmc_display_name = 'MB/SP Cost Code'
WHERE lmc_master_code = 'MB_SPIN'
  AND lmc_column_name = 'mbs_cc';

COMMIT;
