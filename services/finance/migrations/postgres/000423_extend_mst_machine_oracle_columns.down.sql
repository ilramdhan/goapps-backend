ALTER TABLE public.mst_machine
  DROP COLUMN IF EXISTS mc_poy_bobbin_weight,
  DROP COLUMN IF EXISTS mc_tot_fxd_cst,
  DROP COLUMN IF EXISTS mc_bobbin_per_trolly,
  DROP COLUMN IF EXISTS mc_box_cost,
  DROP COLUMN IF EXISTS mc_captive_per_bobbin,
  DROP COLUMN IF EXISTS mc_weightage;
