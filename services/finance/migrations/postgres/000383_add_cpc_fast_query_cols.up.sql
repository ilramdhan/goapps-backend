-- 000383: Add fast-query aggregate columns to cst_product_cost.
-- These are populated at write time from ComputeOutput.ParamSnapshot.
-- NULL = product not yet recalculated since this migration.
-- Source params: COST_CAP_FINAL, COST_DEL_FINAL, VB1-5_DEL_COST from compute.go.

BEGIN;

ALTER TABLE cst_product_cost
    ADD COLUMN IF NOT EXISTS cpc_captive_cost   NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_delivery_cost  NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_vb1_del_cost   NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_vb2_del_cost   NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_vb3_del_cost   NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_vb4_del_cost   NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS cpc_vb5_del_cost   NUMERIC(20, 6);

COMMENT ON COLUMN cst_product_cost.cpc_captive_cost  IS 'COST_CAP_FINAL from param snapshot — captive packaging cost per kg.';
COMMENT ON COLUMN cst_product_cost.cpc_delivery_cost IS 'COST_DEL_FINAL from param snapshot — delivery packaging cost per kg.';
COMMENT ON COLUMN cst_product_cost.cpc_vb1_del_cost  IS 'VB1_DEL_COST — delivery cost at volume bucket 1 threshold.';
COMMENT ON COLUMN cst_product_cost.cpc_vb2_del_cost  IS 'VB2_DEL_COST — delivery cost at volume bucket 2 threshold.';
COMMENT ON COLUMN cst_product_cost.cpc_vb3_del_cost  IS 'VB3_DEL_COST — delivery cost at volume bucket 3 threshold.';
COMMENT ON COLUMN cst_product_cost.cpc_vb4_del_cost  IS 'VB4_DEL_COST — delivery cost at volume bucket 4 threshold.';
COMMENT ON COLUMN cst_product_cost.cpc_vb5_del_cost  IS 'VB5_DEL_COST — delivery cost at volume bucket 5 threshold.';

-- Partial index: only rows that have been calculated after this migration.
CREATE INDEX IF NOT EXISTS idx_cpc_fast_query
    ON cst_product_cost (cpc_job_id, cpc_period, cpc_delivery_cost)
    WHERE cpc_delivery_cost IS NOT NULL;

COMMIT;
