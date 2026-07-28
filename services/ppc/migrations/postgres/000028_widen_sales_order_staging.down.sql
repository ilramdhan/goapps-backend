-- Revert sales_order_staging column widths to their original sizes.
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_code   TYPE VARCHAR(20);
ALTER TABLE sales_order_staging ALTER COLUMN sos_grade_code      TYPE VARCHAR(20);
ALTER TABLE sales_order_staging ALTER COLUMN sos_shade_code      TYPE VARCHAR(20);
ALTER TABLE sales_order_staging ALTER COLUMN sos_merge_no        TYPE VARCHAR(20);
ALTER TABLE sales_order_staging ALTER COLUMN sos_term            TYPE VARCHAR(20);
ALTER TABLE sales_order_staging ALTER COLUMN sos_contract_no     TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_item_code       TYPE VARCHAR(30);
