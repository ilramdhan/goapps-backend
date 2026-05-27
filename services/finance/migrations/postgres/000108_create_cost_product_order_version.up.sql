-- Canonical PRD Phase B §7.5.2 — cost_product_order_version (CPOV_).
-- One row per version snapshot. status: draft / active / superseded.
-- Partial UNIQUE ensures at most one active version per order.

CREATE TABLE IF NOT EXISTS cost_product_order_version (
    cpov_version_id      BIGSERIAL    PRIMARY KEY,
    cpov_order_id        BIGINT       NOT NULL
        REFERENCES cost_product_order (cpo_order_id) ON DELETE CASCADE,
    cpov_version_no      INT          NOT NULL,
    cpov_status          VARCHAR(20)  NOT NULL,
    cpov_effective_from  TIMESTAMPTZ,
    cpov_effective_to    TIMESTAMPTZ,
    cpov_cycle_override  BOOLEAN      NOT NULL DEFAULT FALSE,
    cpov_created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpov_created_by      VARCHAR(64)  NOT NULL,
    CONSTRAINT chk_cpov_status CHECK (cpov_status IN ('draft', 'active', 'superseded')),
    CONSTRAINT uk_cpov_order_version UNIQUE (cpov_order_id, cpov_version_no)
);

CREATE INDEX IF NOT EXISTS idx_cpov_order_status
    ON cost_product_order_version (cpov_order_id, cpov_status);

-- Partial UNIQUE: at most one active version per order.
CREATE UNIQUE INDEX IF NOT EXISTS uk_cpov_active_per_order
    ON cost_product_order_version (cpov_order_id)
    WHERE cpov_status = 'active';

-- Add FK from cost_product_order.CPO_current_version_id → CPOV_version_id now that the target exists.
ALTER TABLE cost_product_order
    ADD CONSTRAINT fk_cpo_current_version
    FOREIGN KEY (cpo_current_version_id)
    REFERENCES cost_product_order_version (cpov_version_id)
    ON DELETE SET NULL
    DEFERRABLE INITIALLY DEFERRED;

COMMENT ON TABLE cost_product_order_version IS 'PRD Phase B §7.5.2 — Version snapshot of a product order. Each commit creates a new row.';
