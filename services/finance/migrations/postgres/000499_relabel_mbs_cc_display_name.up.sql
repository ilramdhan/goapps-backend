-- 000499: relabel display name of MB_SPIN.mbs_cc lookup-metadata row.
--
-- mbs_cc is business-confirmed to hold shade code data, and the frontend
-- UI is being updated (separate task) to label this column "Shade Code" in
-- forms and list views. The seeded lookup-metadata row registered in
-- 000404_extend_mst_mb_spin_columns.up.sql still carries the old label
-- ('MB/SP Cost Code'), which is now inconsistent with the UI. This migration
-- only updates that single row's display name; the column name, master
-- code, data type, and sort order are untouched.

BEGIN;

UPDATE mst_lookup_master_column
SET lmc_display_name = 'Shade Code'
WHERE lmc_master_code = 'MB_SPIN'
  AND lmc_column_name = 'mbs_cc';

COMMIT;
