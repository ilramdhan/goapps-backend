-- 000549 down — drop only the tables 000549 created (children first).

BEGIN;

DROP TABLE IF EXISTS cst_erp_std_cost;
DROP TABLE IF EXISTS cst_erp_coverage;
DROP TABLE IF EXISTS cst_erp_adj_demand;
DROP TABLE IF EXISTS cst_erp_int_batch;

COMMIT;
