-- Product-machine parameter value layer (v1.2, Opsi A). One definition (mst_parameter),
-- two value grains: cost_product_parameter (per product) + this (per product+machine).
-- pmp_machine_id is a real FK (same DB). product_sys_id + param_id are soft refs (finance).
CREATE TABLE IF NOT EXISTS product_machine_parameter (
    pmp_id                 BIGSERIAL     PRIMARY KEY,
    pmp_cpm_product_sys_id BIGINT        NOT NULL,   -- soft ref finance CPM
    pmp_machine_id         BIGINT        NOT NULL REFERENCES machine(machine_id),
    pmp_param_id           UUID          NOT NULL,    -- soft ref mst_parameter (costing)
    pmp_value_num          DECIMAL(20,6),
    pmp_value_text         TEXT,
    pmp_value_flag         BOOLEAN,
    pmp_updated_at         TIMESTAMPTZ   DEFAULT NOW(),
    CONSTRAINT uq_pmp_product_machine_param UNIQUE (pmp_cpm_product_sys_id, pmp_machine_id, pmp_param_id)
);

CREATE INDEX IF NOT EXISTS idx_pmp_machine_id ON product_machine_parameter (pmp_machine_id);
CREATE INDEX IF NOT EXISTS idx_pmp_product ON product_machine_parameter (pmp_cpm_product_sys_id);
