-- Dynamic workflow engine: instance + instance_step.
-- PRD v1.3 §0.1 D6.

CREATE TABLE IF NOT EXISTS wfl_workflow_instance (
    instance_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id         UUID NOT NULL REFERENCES wfl_workflow_template(template_id),
    template_version    INT NOT NULL,
    entity_kind         VARCHAR(40) NOT NULL,
    entity_id           UUID NOT NULL,
    current_step_no     INT NOT NULL DEFAULT 1,
    status              VARCHAR(20) NOT NULL DEFAULT 'IN_PROGRESS',
    started_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_by          VARCHAR(100) NOT NULL,
    completed_at        TIMESTAMPTZ,
    locked_at           TIMESTAMPTZ,
    locked_by           VARCHAR(100),
    unlocked_at         TIMESTAMPTZ,
    unlocked_by         VARCHAR(100),
    CONSTRAINT chk_wfl_instance_status CHECK (status IN ('IN_PROGRESS','APPROVED','REJECTED','LOCKED','UNLOCKED')),
    CONSTRAINT chk_wfl_instance_entity_kind CHECK (entity_kind IN ('PRD_REQUEST','CST_PRODUCT','PARAM_FILL'))
);

CREATE INDEX IF NOT EXISTS idx_wfl_instance_entity ON wfl_workflow_instance (entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_wfl_instance_status ON wfl_workflow_instance (status) WHERE status = 'IN_PROGRESS';

CREATE TABLE IF NOT EXISTS wfl_workflow_instance_step (
    instance_step_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instance_id                 UUID NOT NULL REFERENCES wfl_workflow_instance(instance_id) ON DELETE CASCADE,
    step_no                     INT NOT NULL,
    step_name                   VARCHAR(200) NOT NULL,
    approver_resolution_type    VARCHAR(10) NOT NULL,
    approver_resolution_value   VARCHAR(200) NOT NULL,
    sla_hours                   INT,
    assigned_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_user_id               UUID,
    decision                    VARCHAR(20),
    decided_at                  TIMESTAMPTZ,
    comment                     TEXT,
    stuck_since                 TIMESTAMPTZ,
    CONSTRAINT chk_wfl_instance_step_decision CHECK (decision IS NULL OR decision IN ('APPROVED','REJECTED','REASSIGNED','SKIPPED')),
    CONSTRAINT uk_wfl_instance_step_no UNIQUE (instance_id, step_no)
);

CREATE INDEX IF NOT EXISTS idx_wfl_instance_step_actor ON wfl_workflow_instance_step (actor_user_id) WHERE actor_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_wfl_instance_step_stuck ON wfl_workflow_instance_step (stuck_since) WHERE stuck_since IS NOT NULL AND decided_at IS NULL;
