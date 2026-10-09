-- 000565: Superba Cost SP master (topic superba-mb-cost-sp).
-- Drives TOP 73 MB_COST_MKT for SUPERBA products (matched by shade code) and the
-- TOP 64 MB_SP_DYE colour-name display. Additive only.
--
-- Unique-key decision: a PLAIN UNIQUE (legacy_sys_id), not a partial index, so
-- `INSERT ... ON CONFLICT (legacy_sys_id)` (seed 000566 and Oracle sync) always
-- has a usable arbiter. Consequence: a soft-deleted row still occupies its
-- legacy_sys_id; upserts revive it (deleted_at = NULL) instead of duplicating.

CREATE TABLE IF NOT EXISTS cost_superba_cost_sp (
    id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    legacy_sys_id BIGINT        NOT NULL,
    shade_code    VARCHAR(30)   NOT NULL,
    colour_name   VARCHAR(200),
    old_value     NUMERIC(20,9) NOT NULL,
    new_value     NUMERIC(20,9),
    source        VARCHAR(10)   NOT NULL DEFAULT 'MANUAL',
    is_active     BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by    VARCHAR(100)  NOT NULL DEFAULT 'system',
    updated_at    TIMESTAMPTZ,
    updated_by    VARCHAR(100),
    deleted_at    TIMESTAMPTZ,
    deleted_by    VARCHAR(100),
    CONSTRAINT uk_cost_superba_cost_sp_legacy_sys_id UNIQUE (legacy_sys_id),
    CONSTRAINT chk_cost_superba_cost_sp_source CHECK (source IN ('MANUAL', 'ORACLE', 'SEED'))
);

-- Shade resolution lookup (calc loader / display / "effective" flag).
CREATE INDEX IF NOT EXISTS idx_cost_superba_cost_sp_shade_norm
    ON cost_superba_cost_sp (UPPER(TRIM(shade_code)), legacy_sys_id DESC)
    WHERE deleted_at IS NULL AND is_active;

COMMENT ON TABLE cost_superba_cost_sp IS 'Superba Cost SP master: per-shade MB cost marketing (old_value) and colour name for SUPERBA products. Seeded from CSV, upserted by legacy_sys_id from Oracle legacy.';
COMMENT ON COLUMN cost_superba_cost_sp.legacy_sys_id IS 'Legacy (Oracle) Sys Id; upsert key.';
COMMENT ON COLUMN cost_superba_cost_sp.shade_code IS 'Shade code, stored trimmed; matched to cost_product_master.cpm_shade_code via UPPER(TRIM()).';
COMMENT ON COLUMN cost_superba_cost_sp.colour_name IS 'Superba colour name; shown for TOP 64 MB_SP_DYE (display only).';
COMMENT ON COLUMN cost_superba_cost_sp.old_value IS 'Value used as MB cost marketing (TOP 73 MB_COST_MKT) for SUPERBA products.';
COMMENT ON COLUMN cost_superba_cost_sp.new_value IS 'Informational only; never used in calculation.';
COMMENT ON COLUMN cost_superba_cost_sp.source IS 'Provenance: MANUAL, ORACLE (sync overwrites MANUAL), SEED.';
COMMENT ON COLUMN cost_superba_cost_sp.is_active IS 'Inactive rows are ignored by shade resolution.';
COMMENT ON COLUMN cost_superba_cost_sp.deleted_at IS 'Soft delete marker; deleted rows are ignored by resolution and lists.';
