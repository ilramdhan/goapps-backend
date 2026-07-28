-- Revert ETL-fed string columns to their prior (post-000028) widths.
ALTER TABLE sales_order_staging ALTER COLUMN sos_contract_no     TYPE VARCHAR(100);
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_code   TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_name   TYPE VARCHAR(100);
ALTER TABLE sales_order_staging ALTER COLUMN sos_item_code       TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_grade_code      TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_shade_code      TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_merge_no        TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_term            TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_currency        TYPE VARCHAR(5);
ALTER TABLE sales_order_staging ALTER COLUMN sos_blocked_status  TYPE VARCHAR(50);
