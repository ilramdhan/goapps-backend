-- Revert 000477: de-register exactly the five MB_SPIN columns it added.
-- The physical columns are owned by 000389/000418 and are NOT dropped here.

BEGIN;

DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'MB_SPIN'
  AND lmc_column_name IN ('mbs_denier','mbs_filament','mbs_ldr_prsn','mbs_dozing','mbs_mgt_name');

COMMIT;
