-- Migration: create bi_excel_upload — upload session header (consumed by spec 1C).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_excel_upload (
    upload_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id        UUID NOT NULL REFERENCES bi_data_source(source_id),
    dashboard_id     UUID REFERENCES bi_dashboard(dashboard_id),
    file_name        VARCHAR(255) NOT NULL,
    file_size        INT          NOT NULL,
    file_hash        CHAR(64),
    status           VARCHAR(20)  NOT NULL DEFAULT 'PENDING'
                     CHECK (status IN ('PENDING','VALIDATED','COMMITTING','COMMITTED','FAILED','CANCELLED')),
    total_rows       INT,
    valid_rows       INT,
    invalid_rows     INT,
    overwrite_rows   INT,
    committed_rows   INT,
    error_summary    JSONB,
    minio_object_key VARCHAR(500),
    uploaded_by      UUID NOT NULL,
    uploaded_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    committed_at     TIMESTAMP,
    cancelled_at     TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bi_eu_status ON bi_excel_upload (status, uploaded_at DESC);
CREATE INDEX IF NOT EXISTS idx_bi_eu_user   ON bi_excel_upload (uploaded_by, uploaded_at DESC);

COMMIT;
