-- Migration: create bi_job_log — per-run audit trail for ETL jobs.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_job_log (
    log_id        BIGSERIAL PRIMARY KEY,
    job_id        UUID NOT NULL REFERENCES bi_job(job_id),
    started_at    TIMESTAMP NOT NULL,
    ended_at      TIMESTAMP,
    status        VARCHAR(20) NOT NULL CHECK (status IN ('RUNNING','SUCCESS','FAILED','CANCELLED')),
    rows_affected INT,
    error_message TEXT,
    triggered_by  VARCHAR(80),
    duration_ms   INT
);

CREATE INDEX IF NOT EXISTS idx_bi_jl_job_start ON bi_job_log (job_id, started_at DESC);

COMMIT;
