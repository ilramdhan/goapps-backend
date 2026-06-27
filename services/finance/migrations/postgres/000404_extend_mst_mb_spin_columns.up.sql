ALTER TABLE mst_mb_spin
    ADD COLUMN IF NOT EXISTS mbs_cc             VARCHAR(100),
    ADD COLUMN IF NOT EXISTS mbs_cost_rate_mkt  NUMERIC(15,6);

INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
    ('MB_SPIN', 'mbs_cc',            'MB/SP Cost Code',      'TEXT',   50),
    ('MB_SPIN', 'mbs_cost_rate_mkt', 'MB Rate MKT (USD/kg)', 'NUMBER', 60)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
