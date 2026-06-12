CREATE TABLE IF NOT EXISTS cost_level_assignment_config (
  clac_config_id           BIGSERIAL PRIMARY KEY,
  clac_route_level         INT NOT NULL,
  clac_filler_type         VARCHAR(10) NOT NULL,
  clac_filler_value        VARCHAR(200) NOT NULL,
  clac_approver_type       VARCHAR(10),
  clac_approver_value      VARCHAR(200),
  clac_reapprove_on_change BOOLEAN NOT NULL DEFAULT false,
  clac_sla_fill_hours      INT NOT NULL DEFAULT 48,
  clac_sla_approve_hours   INT NOT NULL DEFAULT 24,
  clac_is_active           BOOLEAN NOT NULL DEFAULT true,
  clac_created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  clac_created_by          VARCHAR(100) NOT NULL,
  clac_updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  clac_updated_by          VARCHAR(100) NOT NULL,
  CONSTRAINT chk_clac_filler_type CHECK (clac_filler_type IN ('USER','DEPT')),
  CONSTRAINT chk_clac_approver_type CHECK (clac_approver_type IS NULL OR clac_approver_type IN ('USER','DEPT'))
);

-- One active config per route level.
CREATE UNIQUE INDEX IF NOT EXISTS uk_clac_level_active
  ON cost_level_assignment_config (clac_route_level)
  WHERE clac_is_active = true;
