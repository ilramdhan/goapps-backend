-- Migration: Create cst_rm_group_detail — items assigned to an RM group.
-- Rule: one item_code may belong to AT MOST ONE active group (enforced via partial unique index).

CREATE TABLE IF NOT EXISTS cst_rm_group_detail (
    group_detail_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_head_id      UUID NOT NULL REFERENCES cst_rm_group_head(group_head_id) ON DELETE RESTRICT,

    -- Item identification (mirrors cst_item_cons_stk_po).
    item_code          VARCHAR(20) NOT NULL,
    item_name          VARCHAR(200),
    item_type_code     VARCHAR(30),
    grade_code         VARCHAR(40),
    item_grade         VARCHAR(30),
    uom_code           VARCHAR(12),

    -- Marketing breakdown (optional per-item contribution within group).
    market_percentage  DECIMAL(20,6),
    market_value_rp    DECIMAL(20,6),

    -- Ordering + flags.
    sort_order         INT     NOT NULL DEFAULT 0,
    is_active          BOOLEAN NOT NULL DEFAULT true,
    is_dummy           BOOLEAN NOT NULL DEFAULT false,

    -- Audit.
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100) NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100),

    CONSTRAINT chk_rm_group_detail_market_percentage_nonneg CHECK (market_percentage IS NULL OR market_percentage >= 0),
    CONSTRAINT chk_rm_group_detail_market_value_nonneg      CHECK (market_value_rp   IS NULL OR market_value_rp   >= 0)
);

COMMENT ON TABLE cst_rm_group_detail IS 'Items (RMs) assigned to an RM group. One item_code may belong to at most one active group.';

-- One item per active group — enforce "1 item, 1 group" business rule at DB layer.
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_detail_item_active
    ON cst_rm_group_detail (item_code)
    WHERE deleted_at IS NULL AND is_active = true;

-- Fast lookups by group.
CREATE INDEX IF NOT EXISTS idx_rm_group_detail_head
    ON cst_rm_group_detail (group_head_id) WHERE deleted_at IS NULL;

-- Fast lookups by item (e.g., "which group does item X belong to?").
CREATE INDEX IF NOT EXISTS idx_rm_group_detail_item
    ON cst_rm_group_detail (item_code) WHERE deleted_at IS NULL;
