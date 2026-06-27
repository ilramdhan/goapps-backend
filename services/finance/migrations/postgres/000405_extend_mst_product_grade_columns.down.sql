DELETE FROM mst_lookup_master_column
WHERE lmc_master_code = 'PRODUCT_GRADE'
  AND lmc_column_name IN ('pg_detail_product', 'pg_grade_label', 'std_selling_price', 'sp_value');

ALTER TABLE mst_product_grade
    DROP COLUMN IF EXISTS pg_detail_product,
    DROP COLUMN IF EXISTS pg_grade_label,
    DROP COLUMN IF EXISTS std_selling_price,
    DROP COLUMN IF EXISTS sp_value;
