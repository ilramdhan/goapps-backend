CREATE TABLE IF NOT EXISTS cost_request_level_assignment (
  crla_assignment_id       BIGSERIAL PRIMARY KEY,
  crla_request_id          BIGINT NOT NULL REFERENCES cost_product_request(cpr_request_id),
  crla_route_level         INT NOT NULL,
  crla_filler_type         VARCHAR(10),
  crla_filler_value        VARCHAR(200),
  crla_approver_type       VARCHAR(10),
  crla_approver_value      VARCHAR(200),
  crla_reapprove_on_change BOOLEAN,
  crla_sla_fill_hours      INT,
  crla_sla_approve_hours   INT,
  crla_created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  crla_created_by          VARCHAR(100) NOT NULL,
  crla_updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  crla_updated_by          VARCHAR(100) NOT NULL,
  CONSTRAINT uk_crla_request_level UNIQUE (crla_request_id, crla_route_level),
  CONSTRAINT chk_crla_filler_type CHECK (crla_filler_type IS NULL OR crla_filler_type IN ('USER','DEPT')),
  CONSTRAINT chk_crla_approver_type CHECK (crla_approver_type IS NULL OR crla_approver_type IN ('USER','DEPT'))
);
