-- Rollback: drop aud_rm_cost_history and its indexes.

DROP INDEX IF EXISTS idx_aud_rm_cost_group;
DROP INDEX IF EXISTS idx_aud_rm_cost_job;
DROP INDEX IF EXISTS idx_aud_rm_cost_period_rm;

DROP TABLE IF EXISTS aud_rm_cost_history;
