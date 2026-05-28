-- Migration: create bi_audit_log — config-change audit trail for BI dashboards + groups.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_audit_log (
    audit_id     BIGSERIAL PRIMARY KEY,
    entity_type  VARCHAR(20) NOT NULL CHECK (entity_type IN ('dashboard','group')),
    entity_code  VARCHAR(120),
    entity_title VARCHAR(200),
    action       VARCHAR(10) NOT NULL CHECK (action IN ('CREATE','UPDATE','DELETE')),
    changed_by   VARCHAR(120),
    changed_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    summary      TEXT
);

CREATE INDEX IF NOT EXISTS idx_bi_audit_changed_at ON bi_audit_log (changed_at DESC);

COMMIT;
