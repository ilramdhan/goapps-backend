-- 000579 down — drop the per-product RM ratio mode column. New in 000579
-- (NULL = follow product type), so dropping it restores type-only behavior.

BEGIN;

SET LOCAL lock_timeout = '10s';

ALTER TABLE cost_product_master DROP CONSTRAINT IF EXISTS chk_cpm_rm_ratio_mode;

ALTER TABLE cost_product_master DROP COLUMN IF EXISTS cpm_rm_ratio_mode;

COMMIT;
