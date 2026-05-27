-- Dynamic workflow engine: template + template_step.
-- PRD v1.3 §0.1 D6.

CREATE TABLE IF NOT EXISTS wfl_workflow_template (
    template_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         VARCHAR(40) NOT NULL,
    name         VARCHAR(200) NOT NULL,
    version      INT NOT NULL DEFAULT 1,
    is_active    BOOLEAN NOT NULL DEFAULT FALSE,
    description  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   VARCHAR(100) NOT NULL,
    updated_at   TIMESTAMPTZ,
    updated_by   VARCHAR(100),
    deleted_at   TIMESTAMPTZ,
    deleted_by   VARCHAR(100),
    CONSTRAINT chk_wfl_template_kind CHECK (kind IN ('PRODUCT_COSTING','PARAM_FILL'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_wfl_template_kind_version
    ON wfl_workflow_template (kind, version)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uk_wfl_template_kind_active
    ON wfl_workflow_template (kind)
    WHERE is_active = TRUE AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS wfl_workflow_template_step (
    template_step_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id                 UUID NOT NULL REFERENCES wfl_workflow_template(template_id) ON DELETE CASCADE,
    step_no                     INT NOT NULL,
    step_name                   VARCHAR(200) NOT NULL,
    approver_resolution_type    VARCHAR(10) NOT NULL,
    approver_resolution_value   VARCHAR(200) NOT NULL,
    sla_hours                   INT,
    allow_reject                BOOLEAN NOT NULL DEFAULT TRUE,
    allow_reassign              BOOLEAN NOT NULL DEFAULT FALSE,
    require_password_on_unlock  BOOLEAN NOT NULL DEFAULT FALSE,
    reject_to_step_no           INT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by                  VARCHAR(100) NOT NULL,
    CONSTRAINT chk_wfl_step_resolution CHECK (approver_resolution_type IN ('ROLE','USER','DEPT')),
    CONSTRAINT uk_wfl_template_step_no UNIQUE (template_id, step_no)
);

CREATE INDEX IF NOT EXISTS idx_wfl_template_step_template ON wfl_workflow_template_step (template_id);
