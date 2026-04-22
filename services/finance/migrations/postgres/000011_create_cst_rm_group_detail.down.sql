-- Rollback: drop cst_rm_group_detail and all its indexes.

DROP INDEX IF EXISTS idx_rm_group_detail_item;
DROP INDEX IF EXISTS idx_rm_group_detail_head;
DROP INDEX IF EXISTS uk_rm_group_detail_item_active;

DROP TABLE IF EXISTS cst_rm_group_detail;
