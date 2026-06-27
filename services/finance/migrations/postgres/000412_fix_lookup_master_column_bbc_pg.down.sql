BEGIN;

DELETE FROM public.mst_lookup_master_column
WHERE (lmc_master_code = 'BOX_BOBBIN_COST' AND lmc_column_name IN ('bbn_reuse','box_reuse','box_cost','bobin_cost','box_cost_val','bobin_cost_val'))
   OR (lmc_master_code = 'PRODUCT_GRADE'   AND lmc_column_name IN ('loss_pct','seq_no'));

-- Restore stale entries (were there before 000412)
INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('BOX_BOBBIN_COST', 'bbcr_bob_rate_mkt', 'Bobbin Rate MKT (USD/bob)', 'NUMBER', 20),
  ('BOX_BOBBIN_COST', 'bbcr_box_rate_mkt', 'Box Rate MKT (USD/box)',    'NUMBER', 30)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
