-- Migration: Create general-purpose job execution tracking tables.
-- Used by: Oracle sync, calculation costing, exports, etc.

CREATE TABLE IF NOT EXISTS job_execution (
    -- Primary key.
    job_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Job identification.
    job_code        VARCHAR(30) NOT NULL UNIQUE,
    job_type        VARCHAR(50) NOT NULL,
    job_subtype     VARCHAR(50),

    -- Context.
    period          VARCHAR(20),
    status          VARCHAR(20) NOT NULL DEFAULT 'QUEUED',
    priority        INT NOT NULL DEFAULT 5,

    -- Parameters and result.
    params          JSONB,
    result_summary  JSONB,
    error_message   TEXT,

    -- Progress tracking.
    progress        INT NOT NULL DEFAULT 0,
    retry_count     INT NOT NULL DEFAULT 0,
    max_retries     INT NOT NULL DEFAULT 3,

    -- Timestamps.
    queued_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,

    -- Ownership.
    created_by      VARCHAR(100) NOT NULL,
    cancelled_by    VARCHAR(100),
    cancelled_at    TIMESTAMPTZ,

    -- Constraints.
    CONSTRAINT chk_job_status CHECK (status IN ('QUEUED', 'PROCESSING', 'SUCCESS', 'FAILED', 'CANCELLED')),
    CONSTRAINT chk_job_priority CHECK (priority BETWEEN 1 AND 10),
    CONSTRAINT chk_job_progress CHECK (progress BETWEEN 0 AND 100)
);

COMMENT ON TABLE job_execution IS 'General-purpose job execution tracking for async operations.';

-- Indexes for common query patterns.
CREATE INDEX IF NOT EXISTS idx_job_execution_type ON job_execution(job_type);
CREATE INDEX IF NOT EXISTS idx_job_execution_status ON job_execution(status);
CREATE INDEX IF NOT EXISTS idx_job_execution_period ON job_execution(period);
CREATE INDEX IF NOT EXISTS idx_job_execution_queued_at ON job_execution(queued_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_execution_type_period ON job_execution(job_type, period);

-- Prevent duplicate active jobs for the same type and period.
CREATE UNIQUE INDEX IF NOT EXISTS idx_job_execution_active_unique
    ON job_execution(job_type, period)
    WHERE status IN ('QUEUED', 'PROCESSING');

-- Job execution log tracks steps within a single job.
CREATE TABLE IF NOT EXISTS job_execution_log (
    log_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES job_execution(job_id) ON DELETE CASCADE,

    -- Step identification.
    step            VARCHAR(100) NOT NULL,
    status          VARCHAR(20) NOT NULL,
    message         TEXT,
    metadata        JSONB,

    -- Timing.
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    duration_ms     INT,

    -- Constraints.
    CONSTRAINT chk_log_status CHECK (status IN ('STARTED', 'SUCCESS', 'FAILED', 'SKIPPED'))
);

COMMENT ON TABLE job_execution_log IS 'Step-by-step log entries for job execution tracking.';

CREATE INDEX IF NOT EXISTS idx_job_execution_log_job ON job_execution_log(job_id);
CREATE INDEX IF NOT EXISTS idx_job_execution_log_job_step ON job_execution_log(job_id, step);
