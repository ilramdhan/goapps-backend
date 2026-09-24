-- 000520 down — drop the Oil Group flag from RM group heads.
-- v_rm_group_oil (000522) depends on this column; 000522's down drops the view
-- first, so running the downs in reverse order is always safe.

BEGIN;

DROP INDEX IF EXISTS idx_rm_group_head_is_oil_group;

ALTER TABLE cst_rm_group_head
    DROP COLUMN IF EXISTS is_oil_group;

COMMIT;
