-- Widen sales_order_staging string columns fed by the SO ETL. Real Oracle
-- MGT_SO_PENDING_WEB data overflowed VARCHAR(20) (SQLSTATE 22001, value too long).
-- The source columns have no strict width guarantee, so widen defensively.
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_code   TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_grade_code      TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_shade_code      TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_merge_no        TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_term            TYPE VARCHAR(50);
ALTER TABLE sales_order_staging ALTER COLUMN sos_contract_no     TYPE VARCHAR(100);
ALTER TABLE sales_order_staging ALTER COLUMN sos_item_code       TYPE VARCHAR(50);
