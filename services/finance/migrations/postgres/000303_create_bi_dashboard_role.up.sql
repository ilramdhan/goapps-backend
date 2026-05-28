-- Migration: create bi_dashboard_role — fine-grain access whitelist per dashboard.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_dashboard_role (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id UUID NOT NULL REFERENCES bi_dashboard(dashboard_id) ON DELETE CASCADE,
    role_code    VARCHAR(60) NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by   UUID,
    CONSTRAINT uq_bi_dr UNIQUE (dashboard_id, role_code)
);

CREATE INDEX IF NOT EXISTS idx_bi_dr_role ON bi_dashboard_role (role_code);

COMMIT;
