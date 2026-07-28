-- Product-machine capacity (v1.2: planning-math only). Per (product, machine)
-- planning throughput + target efficiency. speed/positions/draw_ratio moved to
-- product_machine_parameter. pmc_cpm_product_sys_id is a soft ref to finance CPM.
CREATE TABLE IF NOT EXISTS product_machine_capacity (
    pmc_id                 BIGSERIAL     PRIMARY KEY,
    pmc_cpm_product_sys_id BIGINT        NOT NULL,
    pmc_machine_id         BIGINT        NOT NULL,
    pmc_prod_per_day       DECIMAL(10,3),             -- planning capacity
    pmc_efficiency_pct     DECIMAL(5,2),              -- planning target eff (not actual)
    pmc_created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    pmc_created_by         VARCHAR(100)  NOT NULL DEFAULT 'system',
    pmc_updated_at         TIMESTAMPTZ,
    pmc_updated_by         VARCHAR(100),
    CONSTRAINT uq_pmc_product_machine UNIQUE (pmc_cpm_product_sys_id, pmc_machine_id)
);

CREATE INDEX IF NOT EXISTS idx_pmc_machine_id ON product_machine_capacity (pmc_machine_id);
