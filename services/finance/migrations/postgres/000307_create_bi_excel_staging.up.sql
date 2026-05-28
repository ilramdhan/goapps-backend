-- Migration: create bi_excel_staging — per-row validation buffer (consumed by spec 1C).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_excel_staging (
    staging_id        BIGSERIAL PRIMARY KEY,
    upload_id         UUID NOT NULL REFERENCES bi_excel_upload(upload_id) ON DELETE CASCADE,
    row_number        INT  NOT NULL,
    type              VARCHAR(40),
    group_1           VARCHAR(120),
    group_2           VARCHAR(120),
    group_3           VARCHAR(120),
    periode_grain     VARCHAR(10),
    periode_date      DATE,
    value             NUMERIC(20,4),
    uom               VARCHAR(20),
    validation_status VARCHAR(20)  NOT NULL CHECK (validation_status IN ('VALID','INVALID','WILL_OVERWRITE')),
    validation_msg    TEXT,
    existing_value    NUMERIC(20,4)
);

CREATE INDEX IF NOT EXISTS idx_bi_es_upload ON bi_excel_staging (upload_id, validation_status, row_number);

COMMIT;
