CREATE TABLE IF NOT EXISTS cost_fill_task (
  cft_task_id             BIGSERIAL PRIMARY KEY,
  cft_request_id          BIGINT NOT NULL REFERENCES cost_product_request(cpr_request_id),
  cft_route_head_id       BIGINT NOT NULL REFERENCES cost_route_head(crh_head_id),
  cft_route_level         INT NOT NULL,
  cft_filler_type         VARCHAR(10) NOT NULL,
  cft_filler_value        VARCHAR(200) NOT NULL,
  cft_approver_type       VARCHAR(10),
  cft_approver_value      VARCHAR(200),
  cft_reapprove_on_change BOOLEAN NOT NULL DEFAULT false,
  cft_sla_fill_hours      INT NOT NULL DEFAULT 48,
  cft_sla_approve_hours   INT NOT NULL DEFAULT 24,
  cft_status              VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
  cft_total_params        INT NOT NULL DEFAULT 0,
  cft_filled_params       INT NOT NULL DEFAULT 0,
  cft_claimed_by          VARCHAR(64),
  cft_claimed_at          TIMESTAMPTZ,
  cft_filled_at           TIMESTAMPTZ,
  cft_activated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cft_last_notified_at    TIMESTAMPTZ,
  cft_created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT uk_cft_request_level UNIQUE (cft_request_id, cft_route_level),
  CONSTRAINT chk_cft_status CHECK (cft_status IN
    ('ACTIVE','FILLING','FILLED','APPROVAL_PENDING','APPROVED','REJECTED')),
  CONSTRAINT chk_cft_filler_type CHECK (cft_filler_type IN ('USER','DEPT'))
);

CREATE INDEX IF NOT EXISTS idx_cft_request ON cost_fill_task (cft_request_id);
CREATE INDEX IF NOT EXISTS idx_cft_status ON cost_fill_task (cft_status)
  WHERE cft_status <> 'APPROVED';
CREATE INDEX IF NOT EXISTS idx_cft_sla ON cost_fill_task (cft_activated_at, cft_sla_fill_hours)
  WHERE cft_status IN ('ACTIVE','FILLING');
