-- Canonical PRD Phase B §7.5.3 — cost_product_order_component (CPOC_).
-- Single-level BOM rows. Dual FK governed by cost_rm_type.CRMT_reference_target:
--   PRODUCT → CPOC_rm_product_sys_id (cost_product_master)
--   MASTER  → CPOC_rm_master_item_id (cost_erp_item)
-- Free-text fallback CPOC_rm_description allowed when RM not yet in master.
-- Multi-Yarn-like types use sub_sequence to allow multiple rows per sequence_no.

CREATE TABLE IF NOT EXISTS cost_product_order_component (
    cpoc_component_id        BIGSERIAL    PRIMARY KEY,
    cpoc_version_id          BIGINT       NOT NULL
        REFERENCES cost_product_order_version (cpov_version_id) ON DELETE CASCADE,
    cpoc_sequence_no         INT          NOT NULL,
    cpoc_sub_sequence        INT,
    cpoc_sub_type            VARCHAR(30),
    cpoc_rm_type_id          INT          NOT NULL
        REFERENCES cost_rm_type (crmt_type_id) ON DELETE RESTRICT,
    cpoc_rm_product_sys_id   BIGINT
        REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE RESTRICT,
    cpoc_rm_master_item_id   BIGINT
        REFERENCES cost_erp_item (cei_item_id) ON DELETE RESTRICT,
    cpoc_rm_description      VARCHAR(255),
    CONSTRAINT chk_cpoc_rm_ref_one CHECK (
        (cpoc_rm_product_sys_id IS NOT NULL)::INT
      + (cpoc_rm_master_item_id IS NOT NULL)::INT
      <= 1
    ),
    CONSTRAINT chk_cpoc_rm_ref_present CHECK (
        cpoc_rm_product_sys_id IS NOT NULL
     OR cpoc_rm_master_item_id IS NOT NULL
     OR cpoc_rm_description    IS NOT NULL
    ),
    CONSTRAINT uk_cpoc_seq UNIQUE (cpoc_version_id, cpoc_sequence_no, cpoc_sub_sequence)
);

CREATE INDEX IF NOT EXISTS idx_cpoc_version
    ON cost_product_order_component (cpoc_version_id, cpoc_sequence_no);

CREATE INDEX IF NOT EXISTS idx_cpoc_rm_product
    ON cost_product_order_component (cpoc_rm_product_sys_id)
    WHERE cpoc_rm_product_sys_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cpoc_rm_master
    ON cost_product_order_component (cpoc_rm_master_item_id)
    WHERE cpoc_rm_master_item_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cpoc_rm_type
    ON cost_product_order_component (cpoc_rm_type_id);

COMMENT ON TABLE cost_product_order_component IS 'PRD Phase B §7.5.3 — Single-level BOM line. Dual FK with CHECK: at most one of rm_product_sys_id / rm_master_item_id, at least one of (those + rm_description) populated.';
