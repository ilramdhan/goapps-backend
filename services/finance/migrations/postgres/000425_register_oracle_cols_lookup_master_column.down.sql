BEGIN;
DELETE FROM public.mst_lookup_master_column
WHERE (lmc_master_code = 'MACHINE' AND lmc_column_name IN (
  'mc_poy_bobbin_weight','mc_tot_fxd_cst','mc_bobbin_per_trolly',
  'mc_box_cost','mc_captive_per_bobbin','mc_weightage'))
  OR (lmc_master_code = 'BOX_BOBBIN_COST' AND lmc_column_name IN ('bbn_reuse_val','box_reuse_val'));
COMMIT;
