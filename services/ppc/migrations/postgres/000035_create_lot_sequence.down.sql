-- Revert 000035: drop the lot sequence counter.
--
-- Safe to drop unconditionally: the table holds only the high-water mark per
-- area+year. Already-issued lot numbers live in work_order.wo_lot_no and
-- lot_master, which this migration never touched. Re-applying 000035 restarts
-- the counters at zero, so re-issue would collide with existing lots — that is
-- acceptable for a revert (the collision surfaces as a lot_master primary-key
-- violation, not silent data corruption).

DROP TABLE IF EXISTS lot_sequence;
