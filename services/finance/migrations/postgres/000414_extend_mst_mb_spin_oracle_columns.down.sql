ALTER TABLE public.mst_mb_spin
  DROP COLUMN IF EXISTS mbs_run_ldr_pct,
  DROP COLUMN IF EXISTS mbs_mb_spg_orion,
  DROP COLUMN IF EXISTS mbs_lesture,
  DROP COLUMN IF EXISTS mbs_d_f,
  DROP COLUMN IF EXISTS mbs_vs_number;

DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'MB_SPIN'
  AND lmc_column_name IN ('mbs_run_ldr_pct','mbs_lesture','mbs_d_f','mbs_mb_spg_orion','mbs_vs_number');
