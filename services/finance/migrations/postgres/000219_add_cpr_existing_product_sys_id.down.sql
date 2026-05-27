DROP INDEX IF EXISTS idx_cpr_existing_product;
ALTER TABLE cost_product_request DROP COLUMN IF EXISTS cpr_existing_product_sys_id;
