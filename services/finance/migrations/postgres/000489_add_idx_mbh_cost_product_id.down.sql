-- Revert K-31 (§11 butir 115): drop the partial index on mst_mb_head.mbh_cost_product_id.
-- The column itself is owned by 000445 and is NOT dropped here.
BEGIN;

DROP INDEX IF EXISTS idx_mbh_cost_product_id;

COMMIT;
