-- Migration: create bi_dashboard — config-driven dashboard definition (heart of BI module).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_dashboard (
    dashboard_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_code       VARCHAR(60) UNIQUE NOT NULL,
    dashboard_title      VARCHAR(200) NOT NULL,
    description          TEXT,
    filter_type          VARCHAR(40)  NOT NULL,
    filter_group_1       VARCHAR(100),
    periode_grain        VARCHAR(10)  NOT NULL CHECK (periode_grain IN ('DAILY','MONTHLY','QUARTERLY','YEARLY')),
    default_period       VARCHAR(20)  NOT NULL DEFAULT 'L12M',
    chart_type           VARCHAR(40)  NOT NULL,
    chart_config         JSONB        NOT NULL DEFAULT '{}'::jsonb,
    layout_config        JSONB,
    compare_modes        JSONB        NOT NULL DEFAULT '[]'::jsonb,
    kpi_config           JSONB        NOT NULL DEFAULT '[]'::jsonb,
    drill_enabled        BOOLEAN      NOT NULL DEFAULT TRUE,
    max_drill_level      INT          NOT NULL DEFAULT 3 CHECK (max_drill_level BETWEEN 1 AND 3),
    cache_ttl_sec        INT          NOT NULL DEFAULT 1800 CHECK (cache_ttl_sec BETWEEN 0 AND 86400),
    refresh_interval_sec INT          NOT NULL DEFAULT 0 CHECK (refresh_interval_sec >= 0),
    display_order        INT          NOT NULL DEFAULT 0,
    group_id             UUID REFERENCES bi_dashboard_group(group_id),
    is_active            BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMP    NOT NULL DEFAULT NOW(),
    created_by           UUID,
    updated_at           TIMESTAMP,
    updated_by           UUID,
    deleted_at           TIMESTAMP,
    deleted_by           UUID,
    CONSTRAINT chk_bi_d_code CHECK (dashboard_code ~ '^[A-Z][A-Z0-9_]*$')
);

CREATE INDEX IF NOT EXISTS idx_bi_d_active_order ON bi_dashboard (is_active, group_id, display_order) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bi_d_filter_type  ON bi_dashboard (filter_type) WHERE deleted_at IS NULL;

COMMIT;
