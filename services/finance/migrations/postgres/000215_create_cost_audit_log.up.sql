-- Canonical PRD Phase A §7.1.14 — cost_audit_log (CAL_).
-- Append-only audit trail for sensitive operations. Each row snapshots
-- before/after state for compliance + forensic review.
CREATE TABLE IF NOT EXISTS cost_audit_log (
    cal_log_id       BIGSERIAL    PRIMARY KEY,
    cal_entity_type  VARCHAR(50)  NOT NULL,
    cal_entity_id    BIGINT       NOT NULL,
    cal_operation    VARCHAR(30)  NOT NULL,
    cal_before_data  JSONB,
    cal_after_data   JSONB,
    cal_user_id      VARCHAR(64)  NOT NULL,
    cal_performed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_cal_operation CHECK (
        cal_operation IN (
            'INSERT', 'UPDATE', 'DELETE',
            'STATUS_CHANGE', 'FEASIBILITY', 'CLASSIFICATION_OVERRIDE',
            'ASSIGN', 'PROMOTE', 'HIDE', 'UNHIDE',
            'RULE_CREATE', 'RULE_UPDATE', 'RULE_DELETE'
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_cal_entity
    ON cost_audit_log (cal_entity_type, cal_entity_id, cal_performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_cal_user
    ON cost_audit_log (cal_user_id, cal_performed_at DESC);
CREATE INDEX IF NOT EXISTS idx_cal_performed_at
    ON cost_audit_log (cal_performed_at DESC);

-- Guard rail: forbid UPDATE / DELETE on audit log rows so the table stays append-only.
-- Triggers raise instead of silently dropping mutations.
CREATE OR REPLACE FUNCTION cost_audit_log_forbid_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'cost_audit_log is append-only — % not permitted', TG_OP;
END;
$$;

DROP TRIGGER IF EXISTS cal_no_update ON cost_audit_log;
DROP TRIGGER IF EXISTS cal_no_delete ON cost_audit_log;

CREATE TRIGGER cal_no_update BEFORE UPDATE ON cost_audit_log
    FOR EACH ROW EXECUTE FUNCTION cost_audit_log_forbid_mutation();
CREATE TRIGGER cal_no_delete BEFORE DELETE ON cost_audit_log
    FOR EACH ROW EXECUTE FUNCTION cost_audit_log_forbid_mutation();

COMMENT ON TABLE cost_audit_log IS 'PRD Phase A §7.1.14 — Append-only audit log (triggers enforce immutability).';
