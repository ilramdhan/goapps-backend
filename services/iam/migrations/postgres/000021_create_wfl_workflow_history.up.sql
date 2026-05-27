-- IAM Service Database Migrations
-- 000021: Create workflow history table
--
-- Generic table for auditing all workflow transitions across entities.
-- Uses wfl_ prefix per DB convention for workflow tables.

CREATE TABLE IF NOT EXISTS wfl_workflow_history (
    history_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  VARCHAR(100)  NOT NULL,
    entity_id    UUID          NOT NULL,
    from_state   INTEGER       NOT NULL,
    to_state     INTEGER       NOT NULL,
    action       VARCHAR(50)   NOT NULL,
    user_id      VARCHAR(255)  NOT NULL,
    notes        TEXT          NOT NULL DEFAULT '',
    occurred_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wfl_history_entity
    ON wfl_workflow_history (entity_type, entity_id, occurred_at DESC);

COMMENT ON TABLE wfl_workflow_history IS 'Audit trail for all workflow state transitions';
