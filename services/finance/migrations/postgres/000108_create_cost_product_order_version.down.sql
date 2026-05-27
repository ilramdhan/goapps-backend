ALTER TABLE cost_product_order DROP CONSTRAINT IF EXISTS fk_cpo_current_version;
DROP TABLE IF EXISTS cost_product_order_version;
