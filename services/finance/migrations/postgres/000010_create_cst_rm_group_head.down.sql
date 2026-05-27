-- Rollback: drop cst_rm_group_head and all its indexes/constraints.

DROP INDEX IF EXISTS idx_rm_group_head_search;
DROP INDEX IF EXISTS idx_rm_group_head_is_active;
DROP INDEX IF EXISTS uk_rm_group_head_code_active;

DROP TABLE IF EXISTS cst_rm_group_head;
