CREATE TABLE IF NOT EXISTS cost_product_level_assignment (
  cpla_assignment_id       BIGSERIAL PRIMARY KEY,
  cpla_product_sys_id      BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id),
  cpla_route_level         INT NOT NULL,
  cpla_filler_type         VARCHAR(10),
  cpla_filler_value        VARCHAR(200),
  cpla_approver_type       VARCHAR(10),
  cpla_approver_value      VARCHAR(200),
  cpla_reapprove_on_change BOOLEAN,
  cpla_sla_fill_hours      INT,
  cpla_sla_approve_hours   INT,
  cpla_created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cpla_created_by          VARCHAR(100) NOT NULL,
  cpla_updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cpla_updated_by          VARCHAR(100) NOT NULL,
  CONSTRAINT uk_cpla_product_level UNIQUE (cpla_product_sys_id, cpla_route_level),
  CONSTRAINT chk_cpla_filler_type CHECK (cpla_filler_type IS NULL OR cpla_filler_type IN ('USER','DEPT')),
  CONSTRAINT chk_cpla_approver_type CHECK (cpla_approver_type IS NULL OR cpla_approver_type IN ('USER','DEPT'))
);
