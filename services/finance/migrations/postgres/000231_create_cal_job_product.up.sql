BEGIN;

CREATE TABLE IF NOT EXISTS cal_job_product (
    cjp_job_product_id  BIGSERIAL PRIMARY KEY,
    cjp_job_id          BIGINT NOT NULL REFERENCES cal_job(cj_job_id) ON DELETE CASCADE,
    cjp_chunk_id        BIGINT REFERENCES cal_job_chunk(cjc_chunk_id),
    cjp_product_sys_id  BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id),
    cjp_route_head_id   BIGINT NOT NULL REFERENCES cost_route_head(crh_head_id),
    cjp_wave_no         INT NOT NULL,
    cjp_status          VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    cjp_block_reason    TEXT,
    cjp_started_at      TIMESTAMPTZ,
    cjp_completed_at    TIMESTAMPTZ,
    cjp_duration_ms     INT,
    cjp_cost_id         BIGINT REFERENCES cst_product_cost(cpc_cost_id),
    cjp_error_message   TEXT,
    cjp_calculation_log JSONB,
    CONSTRAINT uk_cjp_job_product UNIQUE (cjp_job_id, cjp_product_sys_id),
    CONSTRAINT chk_cjp_status CHECK (cjp_status IN ('PENDING','READY','CALCULATING','SUCCESS','FAILED','BLOCKED','SKIPPED'))
);
CREATE INDEX IF NOT EXISTS idx_cjp_job_status     ON cal_job_product (cjp_job_id, cjp_status);
CREATE INDEX IF NOT EXISTS idx_cjp_chunk          ON cal_job_product (cjp_chunk_id);
CREATE INDEX IF NOT EXISTS idx_cjp_product_recent ON cal_job_product (cjp_product_sys_id, cjp_completed_at DESC NULLS LAST);

COMMIT;
