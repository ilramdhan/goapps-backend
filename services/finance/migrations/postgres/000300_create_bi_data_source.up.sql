-- Migration: create bi_data_source — registry of where BI data comes from.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_data_source (
    source_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_code      VARCHAR(40) UNIQUE NOT NULL,
    source_name      VARCHAR(120) NOT NULL,
    source_type      VARCHAR(20)  NOT NULL CHECK (source_type IN ('ORACLE','LARAVEL','EXCEL','MANUAL','API')),
    connection_info  JSONB,
    description      TEXT,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by       UUID,
    updated_at       TIMESTAMP,
    updated_by       UUID,
    CONSTRAINT chk_bi_ds_code CHECK (source_code ~ '^[A-Z][A-Z0-9_]*$')
);

CREATE INDEX IF NOT EXISTS idx_bi_ds_active ON bi_data_source (is_active);

COMMIT;
