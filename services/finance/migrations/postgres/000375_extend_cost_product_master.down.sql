ALTER TABLE cost_product_master
  DROP COLUMN IF EXISTS cpm_shade_name,
  DROP COLUMN IF EXISTS cpm_flex_01,
  DROP COLUMN IF EXISTS cpm_flex_02,
  DROP COLUMN IF EXISTS cpm_flex_03;
