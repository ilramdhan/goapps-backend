-- Rollback: remove only the rows this backfill inserted (is_backfilled = true).
-- Real per-period edits made after the backfill (is_backfilled = false) are
-- left untouched.

DELETE FROM cst_rm_group_detail_period WHERE is_backfilled = true;
DELETE FROM cst_rm_group_head_period WHERE is_backfilled = true;
