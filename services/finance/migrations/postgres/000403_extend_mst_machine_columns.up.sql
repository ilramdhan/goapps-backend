ALTER TABLE mst_machine
    ADD COLUMN IF NOT EXISTS mp_per_day      NUMERIC(15,4),
    ADD COLUMN IF NOT EXISTS ohs_per_day     NUMERIC(15,4),
    ADD COLUMN IF NOT EXISTS spares_per_day  NUMERIC(15,4),
    ADD COLUMN IF NOT EXISTS kgs_lost_change NUMERIC(10,4),
    ADD COLUMN IF NOT EXISTS vb1_qty         NUMERIC(15,2),
    ADD COLUMN IF NOT EXISTS vb2_qty         NUMERIC(15,2),
    ADD COLUMN IF NOT EXISTS vb3_qty         NUMERIC(15,2),
    ADD COLUMN IF NOT EXISTS vb4_qty         NUMERIC(15,2),
    ADD COLUMN IF NOT EXISTS vb5_qty         NUMERIC(15,2);

-- Register new fillable columns in mst_lookup_master_column
INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
    ('MACHINE', 'mp_per_day',      'Manpower Per Day (USD)',        'NUMBER', 70),
    ('MACHINE', 'ohs_per_day',     'Overhead Per Head (USD/day)',   'NUMBER', 80),
    ('MACHINE', 'spares_per_day',  'Spares Cost Per Day (USD)',     'NUMBER', 90),
    ('MACHINE', 'kgs_lost_change', 'Change-Over Quality Loss (kg)', 'NUMBER', 100),
    ('MACHINE', 'vb1_qty',         'Volume Bucket 1 Qty',          'NUMBER', 110),
    ('MACHINE', 'vb2_qty',         'Volume Bucket 2 Qty',          'NUMBER', 120),
    ('MACHINE', 'vb3_qty',         'Volume Bucket 3 Qty',          'NUMBER', 130),
    ('MACHINE', 'vb4_qty',         'Volume Bucket 4 Qty',          'NUMBER', 140),
    ('MACHINE', 'vb5_qty',         'Volume Bucket 5 Qty',          'NUMBER', 150)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
