-- cost_product_applicable_param (CAPP_) — per-product param subset selection.
--
-- Replaces the GLOBAL mst_parameter.is_required_for_costing semantics:
--   - Each product carries its OWN list of applicable parameters.
--   - Each (product, param) pair has its own is_required flag (default copied
--     from mst_parameter.is_required_for_costing when added).
--   - cost_product_parameter (value) rows are only writable for params that
--     appear in CAPP for the product.
--
-- Per user direction (Opsi B): manual per-product selection now; templates +
-- product duplication are future convenience layers on top of this.

CREATE TABLE IF NOT EXISTS cost_product_applicable_param (
    capp_id              BIGSERIAL PRIMARY KEY,
    capp_product_sys_id  BIGINT       NOT NULL
        REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE CASCADE,
    capp_param_id        UUID         NOT NULL
        REFERENCES mst_parameter(id),
    -- Per-product override of mst_parameter.is_required_for_costing.
    capp_is_required     BOOLEAN      NOT NULL DEFAULT FALSE,
    -- Optional per-product render order override (NULL = use mst_parameter.display_order).
    capp_display_order   INT,

    capp_created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    capp_created_by      VARCHAR(100) NOT NULL,
    capp_updated_at      TIMESTAMPTZ,
    capp_updated_by      VARCHAR(100),

    CONSTRAINT capp_unique_product_param UNIQUE (capp_product_sys_id, capp_param_id)
);

CREATE INDEX IF NOT EXISTS idx_capp_product
    ON cost_product_applicable_param(capp_product_sys_id);
CREATE INDEX IF NOT EXISTS idx_capp_required
    ON cost_product_applicable_param(capp_product_sys_id, capp_is_required)
    WHERE capp_is_required = TRUE;

COMMENT ON TABLE  cost_product_applicable_param IS
    'Per-product subset of mst_parameter. A product only sees params present here.';
COMMENT ON COLUMN cost_product_applicable_param.capp_is_required IS
    'Per-product override: TRUE means this param must be filled before the request can leave PARAMETER_PENDING for this product.';
COMMENT ON COLUMN cost_product_applicable_param.capp_display_order IS
    'When NULL, fallback to mst_parameter.display_order. Allows per-product reorder.';
