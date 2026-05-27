-- Canonical PRD Phase B §7.5.1 — cost_product_order (CPO_).
-- "BOM definition for a product" — separate from product identity (CPM_).
-- One product master can have at most one active product order.
-- CPO_current_version_id is set after first commit (see migration 108).

CREATE TABLE IF NOT EXISTS cost_product_order (
    cpo_order_id            BIGSERIAL    PRIMARY KEY,
    cpo_product_sys_id      BIGINT       NOT NULL
        REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE RESTRICT,
    cpo_cyl_type_id         INT,
    cpo_current_version_id  BIGINT,  -- FK added after cost_product_order_version exists (108)
    cpo_is_active           BOOLEAN      NOT NULL DEFAULT TRUE,
    cpo_created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpo_created_by          VARCHAR(64)  NOT NULL,
    cpo_updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpo_updated_by          VARCHAR(64)  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cost_product_order_master
    ON cost_product_order (cpo_product_sys_id);

-- One active product_order per product_master.
CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_product_order_active_per_master
    ON cost_product_order (cpo_product_sys_id)
    WHERE cpo_is_active = TRUE;

COMMENT ON TABLE cost_product_order IS 'PRD Phase B §7.5.1 — BOM definition for a product. Versions live in cost_product_order_version.';
