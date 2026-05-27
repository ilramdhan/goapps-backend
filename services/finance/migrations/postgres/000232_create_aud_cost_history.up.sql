BEGIN;

CREATE TABLE IF NOT EXISTS aud_cost_history (
    ach_history_id      BIGSERIAL PRIMARY KEY,
    ach_product_sys_id  BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id),
    ach_period          VARCHAR(6) NOT NULL,
    ach_calc_type       VARCHAR(10) NOT NULL,
    ach_old_cost_id     BIGINT REFERENCES cst_product_cost(cpc_cost_id),
    ach_new_cost_id     BIGINT REFERENCES cst_product_cost(cpc_cost_id),
    ach_old_total       NUMERIC(20,6),
    ach_new_total       NUMERIC(20,6),
    ach_variance_pct    NUMERIC(10,4),
    ach_old_job_id      BIGINT REFERENCES cal_job(cj_job_id),
    ach_new_job_id      BIGINT REFERENCES cal_job(cj_job_id),
    ach_change_reason   VARCHAR(50),
    ach_changed_by      VARCHAR(100) NOT NULL,
    ach_changed_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ach_product_period ON aud_cost_history (ach_product_sys_id, ach_period DESC, ach_changed_at DESC);
CREATE INDEX IF NOT EXISTS idx_ach_job            ON aud_cost_history (ach_new_job_id);
CREATE INDEX IF NOT EXISTS idx_ach_variance       ON aud_cost_history (ach_period, ach_variance_pct DESC) WHERE ach_variance_pct > 5.0;

COMMIT;
