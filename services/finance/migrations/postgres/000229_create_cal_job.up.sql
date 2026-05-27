BEGIN;

CREATE TABLE IF NOT EXISTS cal_job (
    cj_job_id            BIGSERIAL PRIMARY KEY,
    cj_job_code          VARCHAR(40) NOT NULL UNIQUE,
    cj_period            VARCHAR(6) NOT NULL,
    cj_calculation_type  VARCHAR(10) NOT NULL,
    cj_scope             VARCHAR(20) NOT NULL,
    cj_product_filter    JSONB,
    cj_status            VARCHAR(20) NOT NULL DEFAULT 'QUEUED',
    cj_priority          INT NOT NULL DEFAULT 5,
    cj_total_products    INT NOT NULL DEFAULT 0,
    cj_total_chunks      INT NOT NULL DEFAULT 0,
    cj_total_waves       INT NOT NULL DEFAULT 0,
    cj_processed_chunks  INT NOT NULL DEFAULT 0,
    cj_success_count     INT NOT NULL DEFAULT 0,
    cj_failed_count      INT NOT NULL DEFAULT 0,
    cj_blocked_count     INT NOT NULL DEFAULT 0,
    cj_error_summary     JSONB,
    cj_triggered_by      VARCHAR(20) NOT NULL DEFAULT 'MANUAL',
    cj_queued_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    cj_started_at        TIMESTAMPTZ,
    cj_completed_at      TIMESTAMPTZ,
    cj_duration_ms       BIGINT,
    cj_created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    cj_created_by        VARCHAR(100) NOT NULL,
    CONSTRAINT chk_cj_status CHECK (cj_status IN ('QUEUED','PLANNING','PROCESSING','SUCCESS','PARTIAL_FAILED','FAILED','CANCELLED')),
    CONSTRAINT chk_cj_calc_type CHECK (cj_calculation_type IN ('ACTUAL','FORECAST','SELLING')),
    CONSTRAINT chk_cj_scope CHECK (cj_scope IN ('ALL','FILTERED','SINGLE_PRODUCT','SINGLE_ROUTE'))
);
CREATE INDEX IF NOT EXISTS idx_cj_status_priority ON cal_job (cj_status, cj_priority) WHERE cj_status IN ('QUEUED','PLANNING','PROCESSING');
CREATE INDEX IF NOT EXISTS idx_cj_period_type     ON cal_job (cj_period, cj_calculation_type);
CREATE INDEX IF NOT EXISTS idx_cj_created_at      ON cal_job (cj_created_at DESC);

CREATE TABLE IF NOT EXISTS cal_job_code_counter (
    cjcc_year_month  VARCHAR(6) PRIMARY KEY,
    cjcc_last_number INT NOT NULL DEFAULT 0
);

CREATE OR REPLACE FUNCTION generate_cal_job_code(p_clock TIMESTAMPTZ DEFAULT now())
RETURNS VARCHAR AS $$
DECLARE
    v_ym  VARCHAR(6);
    v_n   INT;
    v_max INT;
BEGIN
    v_ym := TO_CHAR(p_clock, 'YYYYMM');
    SELECT COALESCE(MAX(SUBSTRING(cj_job_code FROM '\d+$')::INT), 0)
      INTO v_max
      FROM cal_job WHERE cj_job_code LIKE 'JOB-' || v_ym || '-%';
    INSERT INTO cal_job_code_counter (cjcc_year_month, cjcc_last_number)
    VALUES (v_ym, GREATEST(v_max, 0) + 1)
    ON CONFLICT (cjcc_year_month) DO UPDATE
    SET cjcc_last_number = GREATEST(cal_job_code_counter.cjcc_last_number, EXCLUDED.cjcc_last_number - 1) + 1
    RETURNING cjcc_last_number INTO v_n;
    RETURN 'JOB-' || v_ym || '-' || LPAD(v_n::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION generate_cal_job_code(TIMESTAMPTZ) IS
'Generates JOB-YYYYMM-NNNN codes. Uses MAX(cj_job_code) + cal_job_code_counter row with
GREATEST() self-heal logic — counter row converges with table state if they ever diverge
(e.g. seeded data, manual inserts, or partial rollbacks). Race condition window exists for
the very first concurrent calls of a new month before the counter row is initialized;
consider an advisory lock or moving MAX scan inside the ON CONFLICT RETURNING for strict
atomicity if needed.';

COMMIT;
