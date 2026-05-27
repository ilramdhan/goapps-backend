-- IAM Service Database Migrations
-- 000031: Add employee_level_id and employee_group_id columns to mst_user.

ALTER TABLE mst_user
    ADD COLUMN IF NOT EXISTS employee_level_id UUID,
    ADD COLUMN IF NOT EXISTS employee_group_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_user_employee_level'
    ) THEN
        ALTER TABLE mst_user
            ADD CONSTRAINT fk_user_employee_level
            FOREIGN KEY (employee_level_id)
            REFERENCES mst_employee_level(employee_level_id) ON DELETE SET NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_user_employee_group'
    ) THEN
        ALTER TABLE mst_user
            ADD CONSTRAINT fk_user_employee_group
            FOREIGN KEY (employee_group_id)
            REFERENCES mst_employee_group(employee_group_id) ON DELETE SET NULL;
    END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_user_employee_level ON mst_user(employee_level_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_user_employee_group ON mst_user(employee_group_id) WHERE deleted_at IS NULL;
