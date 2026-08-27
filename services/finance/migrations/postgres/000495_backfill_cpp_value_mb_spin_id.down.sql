-- Reverse 000495 — HONEST NO-OP, documented rather than guessed.
--
-- What the UP migration wrote is indistinguishable, after the fact, from
-- what the live application save path (mb_spin_repository.go) writes into
-- the exact same column for the exact same MB_SPIN parameter: both simply
-- set cost_product_parameter.cpp_value_mb_spin_id to a resolved
-- mst_mb_spin.mbs_id, using the identical exact-match/uniqueness rule, and
-- neither path stamps cpp_updated_at/cpp_updated_by (see the audit-column
-- decision gate in 000495_*.up.sql) or any other marker that would let a
-- query separate "filled by this migration" from "filled by the app after
-- this migration ran".
--
-- The task constraint for this rollback is explicit: it must undo the
-- backfill WITHOUT deleting legitimate save-path resolutions. Given the
-- indistinguishability above, ANY statement that sets
-- cpp_value_mb_spin_id back to NULL for "rows that look backfilled" is
-- necessarily also a guess about which rows those are, and a blanket
-- "SET cpp_value_mb_spin_id = NULL WHERE ... MB_SPIN ..." would erase
-- every save-path resolution written since this migration ran too --
-- exactly the outcome this rollback is required to avoid.
--
-- Therefore this down migration deliberately does NOT touch any data. It
-- exists only so the migration tooling has a paired down file, and to
-- document the trade-off explicitly rather than pretend a safe distinction
-- is possible:
--
--   * If it turns out the 000495 backfill must be undone, the honest
--     options are (a) restore cpp_value_mb_spin_id for affected rows from
--     a pre-migration backup/snapshot taken before 000495 ran, or (b) accept
--     that some now-NULL-again rows will actually be re-derived save-path
--     writes that get wrongly wiped, and decide that is acceptable. Neither
--     of those is a decision this migration file can make on its own
--     authority -- it is a decision gate for whoever runs the rollback.
--
-- 000494's own down migration (DROP COLUMN) remains the only way to fully
-- remove cpp_value_mb_spin_id and everything ever written to it, backfill
-- or save-path alike; that is out of scope for this file, which only owns
-- the 000495 backfill step.

BEGIN;

-- Intentionally no statements: see comment above for why a safe, selective
-- revert of only this migration's writes is not possible.

COMMIT;
