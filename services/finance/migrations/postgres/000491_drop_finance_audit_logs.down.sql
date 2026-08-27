-- Rollback of 000491 is intentionally a NO-OP.
--
-- Recreating the finance-shaped `audit_logs` would reintroduce exactly the
-- collision this migration exists to remove: in the shared `goapps` schema the
-- name belongs to IAM, and a finance-shaped table there breaks IAM's migration
-- 000006 and its live audit trail. The table also held no data worth restoring
-- (nothing in finance ever wrote to it), and 000491 renames rather than drops it
-- whenever rows are present, so a rollback has nothing to recover.
SELECT 1;
