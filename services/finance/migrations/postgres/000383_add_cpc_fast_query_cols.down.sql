BEGIN;

DROP INDEX IF EXISTS idx_cpc_fast_query;

ALTER TABLE cst_product_cost
    DROP COLUMN IF EXISTS cpc_captive_cost,
    DROP COLUMN IF EXISTS cpc_delivery_cost,
    DROP COLUMN IF EXISTS cpc_vb1_del_cost,
    DROP COLUMN IF EXISTS cpc_vb2_del_cost,
    DROP COLUMN IF EXISTS cpc_vb3_del_cost,
    DROP COLUMN IF EXISTS cpc_vb4_del_cost,
    DROP COLUMN IF EXISTS cpc_vb5_del_cost;

COMMIT;
