-- Over-production threshold config. PPC-owned master (seeded). 5-level resolution
-- (WO -> PRODUCT -> PRODUCT_TYPE -> MACHINE_GROUP -> SYSTEM). Unit PCT or DOFF.
CREATE TABLE IF NOT EXISTS overrun_threshold_config (
    otc_id             BIGSERIAL     PRIMARY KEY,
    otc_level          VARCHAR(20)   NOT NULL,   -- SYSTEM/MACHINE_GROUP/PRODUCT_TYPE/PRODUCT/WO
    otc_ref_id         BIGINT,                   -- id of the level target (NULL for SYSTEM)
    otc_threshold_unit CHAR(4)       NOT NULL,   -- PCT / DOFF
    otc_warning_value  DECIMAL(10,3) NOT NULL,
    otc_block_value    DECIMAL(10,3) NOT NULL,
    otc_notes          TEXT,
    otc_is_active      BOOLEAN       NOT NULL DEFAULT TRUE,
    otc_created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    otc_created_by     VARCHAR(100)  NOT NULL DEFAULT 'system',
    otc_updated_at     TIMESTAMPTZ,
    otc_updated_by     VARCHAR(100),
    CONSTRAINT chk_otc_level CHECK (otc_level IN ('SYSTEM', 'MACHINE_GROUP', 'PRODUCT_TYPE', 'PRODUCT', 'WO')),
    CONSTRAINT chk_otc_threshold_unit CHECK (otc_threshold_unit IN ('PCT', 'DOFF'))
);

CREATE INDEX IF NOT EXISTS idx_otc_level_ref ON overrun_threshold_config (otc_level, otc_ref_id) WHERE otc_is_active = TRUE;
