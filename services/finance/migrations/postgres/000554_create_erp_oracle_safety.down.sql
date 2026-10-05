-- 000554 down — drop only the objects 000554 created. The immutability
-- trigger is dropped BEFORE its table (plan-01 P0-T5 acceptance).

BEGIN;

DROP TABLE IF EXISTS cst_erp_oracle_call_log;

DROP TRIGGER IF EXISTS trg_ceas_immutable ON cst_erp_adj_snapshot;
DROP TABLE IF EXISTS cst_erp_adj_snapshot;
DROP FUNCTION IF EXISTS cst_erp_adj_snapshot_forbid_mutation();

DROP TABLE IF EXISTS cst_erp_valuation_preview;

COMMIT;
