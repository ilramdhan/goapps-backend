ALTER TABLE public.mst_box_bobbin_cost
  DROP COLUMN IF EXISTS bbn_reuse_val,
  DROP COLUMN IF EXISTS box_reuse_val;

DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'BOX_BOBBIN_COST'
  AND lmc_column_name IN ('bbn_reuse_val', 'box_reuse_val');
