-- IAM Service Database Migrations
-- 000031 (down): Remove employee_level_id / employee_group_id from mst_user.

DROP INDEX IF EXISTS idx_user_employee_level;
DROP INDEX IF EXISTS idx_user_employee_group;

ALTER TABLE mst_user DROP CONSTRAINT IF EXISTS fk_user_employee_level;
ALTER TABLE mst_user DROP CONSTRAINT IF EXISTS fk_user_employee_group;

ALTER TABLE mst_user
    DROP COLUMN IF EXISTS employee_level_id,
    DROP COLUMN IF EXISTS employee_group_id;
