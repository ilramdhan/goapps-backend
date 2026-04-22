-- Rollback: drop cst_rm_cost and all its indexes.

DROP INDEX IF EXISTS idx_rm_cost_calculated_at;
DROP INDEX IF EXISTS idx_rm_cost_group_head;
DROP INDEX IF EXISTS idx_rm_cost_period;
DROP INDEX IF EXISTS uk_rm_cost_period_rm;

DROP TABLE IF EXISTS cst_rm_cost;
