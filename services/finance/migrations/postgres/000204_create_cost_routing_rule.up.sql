-- Canonical PRD Phase A §7.1.4 — cost_routing_rule (CRR_).
-- Admin-managed rules evaluated first-match on submit. S5 ships the schema only —
-- admin UI deferred to S7 per the 8-session plan.
CREATE TABLE IF NOT EXISTS cost_routing_rule (
    crr_rule_id        SERIAL       PRIMARY KEY,
    crr_priority       INT          NOT NULL,
    crr_condition      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    crr_action_type    VARCHAR(20)  NOT NULL,
    crr_action_target  VARCHAR(100),
    crr_is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    crr_created_by     VARCHAR(64)  NOT NULL,
    crr_created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_crr_action CHECK (crr_action_type IN ('AUTO_ASSIGN', 'TO_TRIAGE'))
);

CREATE INDEX IF NOT EXISTS idx_crr_active_priority
    ON cost_routing_rule (crr_priority)
    WHERE crr_is_active = TRUE;

COMMENT ON TABLE cost_routing_rule IS 'PRD Phase A §7.1.4 — Hybrid routing rule (FR-3). Evaluated first-match-wins by priority.';
