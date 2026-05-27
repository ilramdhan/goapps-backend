-- Extend mst_parameter with 6 canonical metadata columns to support Phase B
-- product↔parameter binding (cost_product_parameter). All columns are additive
-- and nullable/defaulted so existing rows + legacy RM Costing flow remain valid.

ALTER TABLE mst_parameter
    ADD COLUMN IF NOT EXISTS owner_department        VARCHAR(30),
    ADD COLUMN IF NOT EXISTS is_required_for_costing BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_period_dependent     BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS lookup_master_code      VARCHAR(30),
    ADD COLUMN IF NOT EXISTS display_order           INT      NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS display_group           VARCHAR(50);

COMMENT ON COLUMN mst_parameter.owner_department        IS 'Responsible department: Engineering, Production, Finance, RND.';
COMMENT ON COLUMN mst_parameter.is_required_for_costing IS 'TRUE = must be filled per product before request can leave PARAMETER_PENDING.';
COMMENT ON COLUMN mst_parameter.is_period_dependent     IS 'FALSE = stored in cost_product_parameter (Phase B static). TRUE = stored per period in Phase C (deferred).';
COMMENT ON COLUMN mst_parameter.lookup_master_code      IS 'When NOT NULL the UI must render a combobox sourced from the named master (e.g. YARN_TYPE). Free-text fallback while master is not yet built.';
COMMENT ON COLUMN mst_parameter.display_order           IS 'Render order within display_group.';
COMMENT ON COLUMN mst_parameter.display_group           IS 'Form section: Spec / Machine / Grade / Packing / Cost / etc.';

CREATE INDEX IF NOT EXISTS idx_mst_parameter_required_active
    ON mst_parameter(is_required_for_costing)
    WHERE deleted_at IS NULL AND is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_mst_parameter_display_group
    ON mst_parameter(display_group, display_order)
    WHERE deleted_at IS NULL AND is_active = TRUE;
