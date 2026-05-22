BEGIN;

CREATE TABLE IF NOT EXISTS cst_product_cost (
    cpc_cost_id              BIGSERIAL PRIMARY KEY,
    cpc_product_sys_id       BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id),
    cpc_period               VARCHAR(6) NOT NULL,
    cpc_calculation_type     VARCHAR(10) NOT NULL,
    cpc_route_head_id        BIGINT NOT NULL REFERENCES cost_route_head(crh_head_id),
    cpc_version              INT NOT NULL DEFAULT 1,
    cpc_cost_per_unit        NUMERIC(20,6) NOT NULL,
    cpc_total_rm_cost        NUMERIC(20,6),
    cpc_total_conversion     NUMERIC(20,6),
    cpc_total_cost           NUMERIC(20,6),
    cpc_uom_id               INT,
    cpc_currency_code        VARCHAR(3) NOT NULL DEFAULT 'IDR',
    cpc_cost_by_level        JSONB,
    cpc_rm_cost_detail       JSONB,
    cpc_param_snapshot       JSONB,
    cpc_formula_trace        JSONB,
    cpc_input_hash           VARCHAR(64),
    cpc_status               VARCHAR(20) NOT NULL DEFAULT 'CALCULATED',
    cpc_job_id               BIGINT,
    cpc_calculated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpc_calculated_by        VARCHAR(100) NOT NULL,
    cpc_verified_at          TIMESTAMPTZ,
    cpc_verified_by          VARCHAR(100),
    cpc_created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_cpc_status CHECK (cpc_status IN ('CALCULATED','VERIFIED','APPROVED','SUPERSEDED')),
    CONSTRAINT chk_cpc_calc_type CHECK (cpc_calculation_type IN ('ACTUAL','FORECAST','SELLING'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cpc_active
    ON cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type)
    WHERE cpc_status != 'SUPERSEDED';

CREATE INDEX IF NOT EXISTS idx_cpc_period_type_status
    ON cst_product_cost (cpc_period, cpc_calculation_type, cpc_status)
    INCLUDE (cpc_product_sys_id, cpc_cost_per_unit);

CREATE INDEX IF NOT EXISTS idx_cpc_job              ON cst_product_cost (cpc_job_id) WHERE cpc_job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cpc_product_history  ON cst_product_cost (cpc_product_sys_id, cpc_period, cpc_calculation_type, cpc_version DESC);
CREATE INDEX IF NOT EXISTS idx_cpc_route_head       ON cst_product_cost (cpc_route_head_id);

COMMIT;
