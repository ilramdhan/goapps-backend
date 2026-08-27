-- Revert 000478: de-register exactly the one MB_HEAD column it added.
-- The physical column is owned by 000417 and is NOT dropped here.

BEGIN;

DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'MB_HEAD'
  AND lmc_column_name IN ('mbh_ldr_prsn');

COMMIT;
