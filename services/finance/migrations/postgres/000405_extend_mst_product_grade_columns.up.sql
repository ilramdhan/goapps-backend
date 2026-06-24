ALTER TABLE mst_product_grade
    ADD COLUMN IF NOT EXISTS pg_detail_product VARCHAR(100),
    ADD COLUMN IF NOT EXISTS pg_grade_label    VARCHAR(50),
    ADD COLUMN IF NOT EXISTS std_selling_price NUMERIC(10,4) DEFAULT 0,
    ADD COLUMN IF NOT EXISTS sp_value          NUMERIC(10,4) DEFAULT 0;

INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
    ('PRODUCT_GRADE', 'pg_detail_product', 'Detail Product Pattern', 'TEXT',   40),
    ('PRODUCT_GRADE', 'pg_grade_label',    'Grade Label',            'TEXT',   50),
    ('PRODUCT_GRADE', 'std_selling_price', 'Std Selling Price',      'NUMBER', 60),
    ('PRODUCT_GRADE', 'sp_value',          'SP Value',               'NUMBER', 70)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
