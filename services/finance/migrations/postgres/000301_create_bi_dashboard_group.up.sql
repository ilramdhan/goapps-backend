-- Migration: create bi_dashboard_group — left-rail grouping for dashboards.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_dashboard_group (
    group_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_code    VARCHAR(40) UNIQUE NOT NULL,
    group_name    VARCHAR(120) NOT NULL,
    description   TEXT,
    icon          VARCHAR(40),
    display_order INT NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by    UUID,
    updated_at    TIMESTAMP,
    updated_by    UUID,
    CONSTRAINT chk_bi_dg_code CHECK (group_code ~ '^[A-Z][A-Z0-9_]*$')
);

CREATE INDEX IF NOT EXISTS idx_bi_dg_active_order ON bi_dashboard_group (is_active, display_order);

COMMIT;
