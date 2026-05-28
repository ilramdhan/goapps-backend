-- Migration: create bi_job — ETL job registry (real workers wire in spec 1D).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_job (
    job_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_name         VARCHAR(120) UNIQUE NOT NULL,
    source_id        UUID NOT NULL REFERENCES bi_data_source(source_id),
    target_type      VARCHAR(40),
    schedule_cron    VARCHAR(50),
    oracle_procedure VARCHAR(200),
    config           JSONB,
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by       UUID,
    updated_at       TIMESTAMP,
    updated_by       UUID
);

COMMIT;
