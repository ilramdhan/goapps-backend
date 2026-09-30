-- Reverse 000528: drop the TX Weight master.
BEGIN;
DROP INDEX IF EXISTS idx_mst_yarn_tx_weight_type;
DROP INDEX IF EXISTS uix_mst_yarn_tx_weight_type_grade;
DROP TABLE IF EXISTS mst_yarn_tx_weight;
COMMIT;
