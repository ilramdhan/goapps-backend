-- Revert the composite (item_code, grade_code) uniqueness back to item_code
-- only. Dropping rows that would conflict is the caller's responsibility.

DROP INDEX IF EXISTS uk_rm_group_detail_item_grade_active;
DROP INDEX IF EXISTS idx_rm_group_detail_item_grade;

CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_detail_item_active
    ON cst_rm_group_detail (item_code)
    WHERE deleted_at IS NULL AND is_active = true;

CREATE INDEX IF NOT EXISTS idx_rm_group_detail_item
    ON cst_rm_group_detail (item_code) WHERE deleted_at IS NULL;
