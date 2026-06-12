CREATE TABLE IF NOT EXISTS cost_fill_approval (
  cfa_approval_id BIGSERIAL PRIMARY KEY,
  cfa_task_id     BIGINT NOT NULL REFERENCES cost_fill_task(cft_task_id),
  cfa_decision    VARCHAR(10) NOT NULL,
  cfa_decided_by  VARCHAR(64) NOT NULL,
  cfa_decided_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  cfa_note        TEXT,
  cfa_trigger     VARCHAR(20) NOT NULL,
  cfa_created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT chk_cfa_decision CHECK (cfa_decision IN ('APPROVED','REJECTED')),
  CONSTRAINT chk_cfa_trigger CHECK (cfa_trigger IN ('INITIAL','REAPPROVE_ON_CHANGE'))
);

CREATE INDEX IF NOT EXISTS idx_cfa_task ON cost_fill_approval (cfa_task_id, cfa_created_at DESC);
