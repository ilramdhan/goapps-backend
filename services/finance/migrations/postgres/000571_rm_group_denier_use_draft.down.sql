-- 000571 down — remove cst_rm_group_head.denier_use_draft.

BEGIN;

SET LOCAL lock_timeout = '10s';

ALTER TABLE cst_rm_group_head
    DROP COLUMN IF EXISTS denier_use_draft;

COMMIT;
