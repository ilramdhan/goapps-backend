-- sales_order_staging is a full-replace ETL landing table for external Oracle
-- MGT_SO_PENDING_WEB data with no reliable column-width guarantees. Fixed-width
-- VARCHARs keep overflowing (SQLSTATE 22001). Use TEXT for all ETL-fed string
-- columns so any source value lands without truncation.
ALTER TABLE sales_order_staging ALTER COLUMN sos_contract_no     TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_code   TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_customer_name   TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_item_code       TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_grade_code      TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_shade_code      TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_merge_no        TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_term            TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_currency        TYPE TEXT;
ALTER TABLE sales_order_staging ALTER COLUMN sos_blocked_status  TYPE TEXT;
