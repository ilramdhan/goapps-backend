-- 000412: Sync mst_lookup_master_column with actual columns after migration 000411.
--
-- BOX_BOBBIN_COST: remove stale bbcr_* entries (columns never existed),
--                 register the 6 actual Oracle columns added by 000411.
-- PRODUCT_GRADE:  register loss_pct + seq_no added by 000411.

BEGIN;

-- ─── BOX_BOBBIN_COST: remove stale entries ───────────────────────────────────
DELETE FROM public.mst_lookup_master_column
WHERE lmc_master_code = 'BOX_BOBBIN_COST'
  AND lmc_column_name IN ('bbcr_bob_rate_mkt', 'bbcr_box_rate_mkt');

-- ─── BOX_BOBBIN_COST: register actual columns ────────────────────────────────
INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('BOX_BOBBIN_COST', 'bbn_reuse',      'Bobbin Reuse Count',           'NUMBER', 20),
  ('BOX_BOBBIN_COST', 'box_reuse',      'Box Reuse Count',              'NUMBER', 25),
  ('BOX_BOBBIN_COST', 'box_cost',       'Box Rate MKT (USD/box)',        'NUMBER', 40),
  ('BOX_BOBBIN_COST', 'bobin_cost',     'Bobbin Rate MKT (USD/bob)',     'NUMBER', 50),
  ('BOX_BOBBIN_COST', 'box_cost_val',   'Box Rate VAL (USD/box)',        'NUMBER', 60),
  ('BOX_BOBBIN_COST', 'bobin_cost_val', 'Bobbin Rate VAL (USD/bob)',     'NUMBER', 70)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

-- ─── PRODUCT_GRADE: register missing columns ─────────────────────────────────
INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('PRODUCT_GRADE', 'loss_pct', 'Loss Factor',    'NUMBER', 75),
  ('PRODUCT_GRADE', 'seq_no',   'Sequence No.',   'NUMBER', 80)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
