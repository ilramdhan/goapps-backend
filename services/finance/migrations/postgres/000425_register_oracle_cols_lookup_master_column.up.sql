-- 000425: Register new Oracle columns in mst_lookup_master_column.
-- These columns were added via migrations 000421 (BBC) and 000423 (Machine)
-- but not registered for the costing engine lookup system.
BEGIN;

INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('MACHINE', 'mc_poy_bobbin_weight',  'POY Bobbin Weight (kg)',    'NUMBER', 160),
  ('MACHINE', 'mc_tot_fxd_cst',        'Total Fixed Cost',          'NUMBER', 170),
  ('MACHINE', 'mc_bobbin_per_trolly',  'Bobbin Per Trolley',        'NUMBER', 180),
  ('MACHINE', 'mc_box_cost',           'Box Cost',                  'NUMBER', 190),
  ('MACHINE', 'mc_captive_per_bobbin', 'Captive Cost Per Bobbin',   'NUMBER', 200),
  ('MACHINE', 'mc_weightage',          'Machine Weightage',         'NUMBER', 210),
  ('BOX_BOBBIN_COST', 'bbn_reuse_val', 'Bobbin Reuse Count VAL',    'NUMBER', 22),
  ('BOX_BOBBIN_COST', 'box_reuse_val', 'Box Reuse Count VAL',       'NUMBER', 27)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
