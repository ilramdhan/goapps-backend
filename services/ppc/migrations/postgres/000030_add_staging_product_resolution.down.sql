-- 000030 down: remove the staging product-resolution cache.

DROP INDEX IF EXISTS idx_sos_match_status;

ALTER TABLE sales_order_staging
  DROP CONSTRAINT IF EXISTS chk_sos_match_status;

ALTER TABLE sales_order_staging
  DROP COLUMN IF EXISTS sos_matched_at,
  DROP COLUMN IF EXISTS sos_match_count,
  DROP COLUMN IF EXISTS sos_match_status,
  DROP COLUMN IF EXISTS sos_cpm_product_sys_id;
