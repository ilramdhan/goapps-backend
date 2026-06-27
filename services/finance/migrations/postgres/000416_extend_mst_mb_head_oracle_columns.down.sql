ALTER TABLE public.mst_mb_head
  DROP COLUMN IF EXISTS mbh_orion_item_code,
  DROP COLUMN IF EXISTS mbh_mb_spg_orion,
  DROP COLUMN IF EXISTS mbh_run_ldr_pct,
  DROP COLUMN IF EXISTS mbh_d_f,
  DROP COLUMN IF EXISTS mbh_lesture,
  DROP COLUMN IF EXISTS mbh_vs_number,
  DROP COLUMN IF EXISTS notes;

DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'MB_HEAD'
  AND lmc_column_name IN ('mbh_run_ldr_pct','mbh_lesture','mbh_d_f','mbh_mb_spg_orion','mbh_vs_number');
