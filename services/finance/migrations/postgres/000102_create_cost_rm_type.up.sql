-- Canonical PRD Phase B §7.2.3 — cost_rm_type (CRMT_).
-- User-definable master of RM types. reference_target controls which FK is required
-- on cost_product_order_component (PRODUCT → CPOC_rm_product_sys_id, MASTER → CPOC_rm_master_item_id).
-- allow_sub_sequence enables Multi-Yarn-like types.

CREATE TABLE IF NOT EXISTS cost_rm_type (
    crmt_type_id             SERIAL       PRIMARY KEY,
    crmt_type_code           VARCHAR(30)  NOT NULL,
    crmt_type_name           VARCHAR(100) NOT NULL,
    crmt_reference_target    VARCHAR(10)  NOT NULL,
    crmt_allow_sub_sequence  BOOLEAN      NOT NULL DEFAULT FALSE,
    crmt_is_active           BOOLEAN      NOT NULL DEFAULT TRUE,
    crmt_created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_crmt_reference_target
        CHECK (crmt_reference_target IN ('PRODUCT', 'MASTER'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_rm_type_code
    ON cost_rm_type (crmt_type_code);

COMMENT ON TABLE  cost_rm_type IS 'PRD Phase B §7.2.3 — User-definable RM type master. reference_target controls component FK.';
COMMENT ON COLUMN cost_rm_type.crmt_reference_target IS 'PRODUCT = component points to cost_product_master (Captive Cost). MASTER = points to cost_erp_item (Store Rate).';
COMMENT ON COLUMN cost_rm_type.crmt_allow_sub_sequence IS 'When TRUE, multiple components may share the same sequence_no via sub_sequence (e.g. Multi-Yarn).';
