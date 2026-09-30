-- Reverse 000529: remove only the rows this seed inserted (hand-entered rows stay).
BEGIN;
DELETE FROM mst_yarn_tx_weight WHERE created_by = 'seed_000529';
COMMIT;
