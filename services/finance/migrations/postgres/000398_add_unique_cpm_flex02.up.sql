CREATE UNIQUE INDEX IF NOT EXISTS uk_cpm_flex02
  ON cost_product_master (cpm_flex_02)
  WHERE cpm_flex_02 IS NOT NULL AND cpm_flex_02 <> '';
