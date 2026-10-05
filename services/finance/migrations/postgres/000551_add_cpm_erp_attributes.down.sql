-- 000551 down — drop only the columns 000551 added.

BEGIN;

ALTER TABLE cost_product_master
    DROP COLUMN IF EXISTS cpm_erp_prd_per_day,
    DROP COLUMN IF EXISTS cpm_erp_item_type,
    DROP COLUMN IF EXISTS cpm_erp_ms_batch_item,
    DROP COLUMN IF EXISTS cpm_erp_chp_item_code,
    DROP COLUMN IF EXISTS cpm_erp_fg_type;

COMMIT;
