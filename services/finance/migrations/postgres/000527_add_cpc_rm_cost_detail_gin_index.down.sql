BEGIN;

DROP INDEX IF EXISTS idx_cost_product_master_shade_code;
DROP INDEX IF EXISTS idx_cpc_rm_cost_detail_gin;

COMMIT;
