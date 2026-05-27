-- Canonical PRD Phase B §7.3.2 — cost_erp_grade (CEG_).
-- Read-only replica of Oracle ERP grade master (AX/AM/B/C).

CREATE TABLE IF NOT EXISTS cost_erp_grade (
    ceg_grade_id   SERIAL       PRIMARY KEY,
    ceg_grade_code VARCHAR(20)  NOT NULL,
    ceg_grade_name VARCHAR(100),
    ceg_is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    ceg_synced_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_erp_grade_code
    ON cost_erp_grade (ceg_grade_code);

COMMENT ON TABLE cost_erp_grade IS 'PRD Phase B §7.3.2 — Read-only replica of Oracle ERP grade master (AX/AM/B/C).';
