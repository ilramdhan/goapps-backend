BEGIN;

CREATE TABLE IF NOT EXISTS cal_job_chunk (
    cjc_chunk_id        BIGSERIAL PRIMARY KEY,
    cjc_job_id          BIGINT NOT NULL REFERENCES cal_job(cj_job_id) ON DELETE CASCADE,
    cjc_chunk_number    INT NOT NULL,
    cjc_wave_no         INT NOT NULL,
    cjc_product_ids     JSONB NOT NULL,
    cjc_product_count   INT NOT NULL,
    cjc_status          VARCHAR(20) NOT NULL DEFAULT 'QUEUED',
    cjc_worker_id       VARCHAR(100),
    cjc_queued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    cjc_dispatched_at   TIMESTAMPTZ,
    cjc_started_at      TIMESTAMPTZ,
    cjc_completed_at    TIMESTAMPTZ,
    cjc_duration_ms     INT,
    cjc_success_count   INT NOT NULL DEFAULT 0,
    cjc_failed_count    INT NOT NULL DEFAULT 0,
    cjc_error_message   TEXT,
    cjc_retry_count     INT NOT NULL DEFAULT 0,
    cjc_max_retries     INT NOT NULL DEFAULT 3,
    CONSTRAINT uk_cjc_job_chunk UNIQUE (cjc_job_id, cjc_chunk_number),
    CONSTRAINT chk_cjc_status CHECK (cjc_status IN ('QUEUED','DISPATCHED','PROCESSING','SUCCESS','PARTIAL_FAILED','FAILED'))
);
CREATE INDEX IF NOT EXISTS idx_cjc_job_wave ON cal_job_chunk (cjc_job_id, cjc_wave_no, cjc_status);
CREATE INDEX IF NOT EXISTS idx_cjc_status   ON cal_job_chunk (cjc_status) WHERE cjc_status IN ('QUEUED','DISPATCHED','PROCESSING');
CREATE INDEX IF NOT EXISTS idx_cjc_worker   ON cal_job_chunk (cjc_worker_id) WHERE cjc_worker_id IS NOT NULL;

COMMIT;
