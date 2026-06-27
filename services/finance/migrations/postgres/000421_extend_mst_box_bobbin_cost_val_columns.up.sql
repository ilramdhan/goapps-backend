-- 000421: Add VAL reuse columns to mst_box_bobbin_cost (from CST_MST_BOX_BOBIN_COST CSV).
-- CMBBC_BBN_REUSE_VAL and CMBBC_BOX_REUSE_VAL were not in the original seeder.

ALTER TABLE public.mst_box_bobbin_cost
  ADD COLUMN IF NOT EXISTS bbn_reuse_val NUMERIC(15,6),
  ADD COLUMN IF NOT EXISTS box_reuse_val NUMERIC(15,6);

COMMENT ON COLUMN public.mst_box_bobbin_cost.bbn_reuse_val IS 'Oracle CMBBC_BBN_REUSE_VAL — bobbin reuse count for VAL session (13%)';
COMMENT ON COLUMN public.mst_box_bobbin_cost.box_reuse_val IS 'Oracle CMBBC_BOX_REUSE_VAL — box reuse count for VAL session (3%)';

INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('BOX_BOBBIN_COST', 'bbn_reuse_val', 'Bobbin Reuse Count VAL', 'NUMBER', 22),
  ('BOX_BOBBIN_COST', 'box_reuse_val', 'Box Reuse Count VAL',    'NUMBER', 27)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
