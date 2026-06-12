ALTER TABLE cost_product_master
  ADD COLUMN IF NOT EXISTS cpm_shade_name VARCHAR(100),
  ADD COLUMN IF NOT EXISTS cpm_flex_01    VARCHAR(100),
  ADD COLUMN IF NOT EXISTS cpm_flex_02    VARCHAR(20),
  ADD COLUMN IF NOT EXISTS cpm_flex_03    VARCHAR(20);

COMMENT ON COLUMN cost_product_master.cpm_shade_name IS 'Human-readable shade name (e.g. JET BLACK, NATURAL)';
COMMENT ON COLUMN cost_product_master.cpm_flex_01    IS 'Legacy ERP compound key: {erp_item_code}-{shade_code}-{type}';
COMMENT ON COLUMN cost_product_master.cpm_flex_02    IS 'Legacy internal sys_id from old ERP system';
COMMENT ON COLUMN cost_product_master.cpm_flex_03    IS 'Product type code label from legacy system';
