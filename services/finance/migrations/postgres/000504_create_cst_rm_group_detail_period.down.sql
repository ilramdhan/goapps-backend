-- Rollback: drop cst_rm_group_detail_period and all its indexes.

DROP INDEX IF EXISTS idx_rm_group_detail_period_detail;
DROP INDEX IF EXISTS uk_rm_group_detail_period;

DROP TABLE IF EXISTS cst_rm_group_detail_period;
