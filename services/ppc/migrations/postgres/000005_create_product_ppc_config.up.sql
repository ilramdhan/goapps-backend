-- Product PPC config: PPC-side extension of finance CPM (cost_product_master).
-- ppc_cpm_product_sys_id is a SOFT reference (no FK; finance lives in a separate
-- DB) validated at write time via the finance gRPC client.
CREATE TABLE IF NOT EXISTS product_ppc_config (
    ppc_id                 BIGSERIAL     PRIMARY KEY,
    ppc_cpm_product_sys_id BIGINT        NOT NULL UNIQUE,
    ppc_is_commodity_watch BOOLEAN       NOT NULL DEFAULT FALSE,
    ppc_price_sell         DECIMAL(12,2),
    ppc_machine_group_id   BIGINT,
    ppc_yield_std          DECIMAL(5,3),
    ppc_buffer_rm_pct      DECIMAL(5,3),
    ppc_ax_yield_pct       DECIMAL(5,3),  -- historical %AX (0.75-0.84)
    ppc_denier             DECIMAL(8,2),  -- G-005 stopgap: theoretical-calc denier
    ppc_created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    ppc_created_by         VARCHAR(100)  NOT NULL DEFAULT 'system',
    ppc_updated_at         TIMESTAMPTZ,
    ppc_updated_by         VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_product_ppc_config_commodity
    ON product_ppc_config (ppc_is_commodity_watch) WHERE ppc_is_commodity_watch = TRUE;
