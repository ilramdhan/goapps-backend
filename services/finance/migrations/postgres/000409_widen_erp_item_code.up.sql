-- Widen cpm_erp_item_code from VARCHAR(20) to VARCHAR(50) to accommodate
-- Oracle ERP item codes that include descriptive suffixes (e.g. "POY0000350-for selling").
ALTER TABLE cost_product_master
    ALTER COLUMN cpm_erp_item_code TYPE VARCHAR(50);
