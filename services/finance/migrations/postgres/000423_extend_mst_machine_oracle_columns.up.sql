-- 000423: Add Oracle-only columns to mst_machine (from CST_MST_MACHINE CSV).

ALTER TABLE public.mst_machine
  ADD COLUMN IF NOT EXISTS mc_poy_bobbin_weight  NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS mc_tot_fxd_cst        NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS mc_bobbin_per_trolly  NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS mc_box_cost           NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS mc_captive_per_bobbin NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS mc_weightage          NUMERIC(10,4);

COMMENT ON COLUMN public.mst_machine.mc_poy_bobbin_weight  IS 'Oracle CMM_POY_BOBBIN_WEIGHT — kg per bobbin (95%)';
COMMENT ON COLUMN public.mst_machine.mc_tot_fxd_cst        IS 'Oracle CMM_TOT_FXD_CST — total fixed cost (61%)';
COMMENT ON COLUMN public.mst_machine.mc_bobbin_per_trolly  IS 'Oracle CMM_BOBBIN_PER_TROLLY — bobbins per trolley (66%)';
COMMENT ON COLUMN public.mst_machine.mc_box_cost           IS 'Oracle CMM_BOX_COST — box cost per unit (66%)';
COMMENT ON COLUMN public.mst_machine.mc_captive_per_bobbin IS 'Oracle CMM_CAPTIVE_PER_BOBBIN — captive cost per bobbin (66%)';
COMMENT ON COLUMN public.mst_machine.mc_weightage          IS 'Oracle CMM_WEIGHTAGE — machine weightage factor (93%)';
