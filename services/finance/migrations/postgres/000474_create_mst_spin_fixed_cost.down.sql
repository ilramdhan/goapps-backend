-- 000474 down: drop spin fixed-cost pool master.
BEGIN;
DROP TABLE IF EXISTS mst_spin_fixed_cost;
COMMIT;
