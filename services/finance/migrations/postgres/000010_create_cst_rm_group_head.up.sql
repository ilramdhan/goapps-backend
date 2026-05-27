-- Migration: Create cst_rm_group_head — user-defined groups of raw materials (RMs)
-- that share a landed-cost configuration (percentage, per-kg overhead, flags per purpose).

CREATE TABLE IF NOT EXISTS cst_rm_group_head (
    group_head_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identification.
    group_code           VARCHAR(30)  NOT NULL,
    group_name           VARCHAR(200) NOT NULL,
    description          TEXT,
    colourant            VARCHAR(30),
    ci_name              VARCHAR(30),

    -- Cost formula inputs.
    cost_percentage      DECIMAL(20,6) NOT NULL DEFAULT 0,
    cost_per_kg          DECIMAL(20,6) NOT NULL DEFAULT 0,

    -- Flag selects which stage rate is used for each purpose.
    flag_valuation       VARCHAR(20)  NOT NULL DEFAULT 'CONS',
    flag_marketing       VARCHAR(20)  NOT NULL DEFAULT 'CONS',
    flag_simulation      VARCHAR(20)  NOT NULL DEFAULT 'CONS',

    -- INIT override values — used when the corresponding flag is 'INIT'.
    init_val_valuation   DECIMAL(20,6),
    init_val_marketing   DECIMAL(20,6),
    init_val_simulation  DECIMAL(20,6),

    -- Lifecycle.
    is_active            BOOLEAN NOT NULL DEFAULT true,

    -- Audit.
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by           VARCHAR(100) NOT NULL,
    updated_at           TIMESTAMPTZ,
    updated_by           VARCHAR(100),
    deleted_at           TIMESTAMPTZ,
    deleted_by           VARCHAR(100),

    -- Flag value domain.
    CONSTRAINT chk_rm_group_flag_valuation  CHECK (flag_valuation  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_group_flag_marketing  CHECK (flag_marketing  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_group_flag_simulation CHECK (flag_simulation IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),

    -- Non-negative cost inputs.
    CONSTRAINT chk_rm_group_cost_percentage_nonneg CHECK (cost_percentage >= 0),
    CONSTRAINT chk_rm_group_cost_per_kg_nonneg     CHECK (cost_per_kg >= 0),

    -- When a flag is INIT, the corresponding init value MUST be set.
    CONSTRAINT chk_rm_group_init_val_valuation  CHECK (flag_valuation  <> 'INIT' OR init_val_valuation  IS NOT NULL),
    CONSTRAINT chk_rm_group_init_val_marketing  CHECK (flag_marketing  <> 'INIT' OR init_val_marketing  IS NOT NULL),
    CONSTRAINT chk_rm_group_init_val_simulation CHECK (flag_simulation <> 'INIT' OR init_val_simulation IS NOT NULL),

    -- Group code format: uppercase alphanumeric with optional spaces and hyphens,
    -- must start with alphanumeric, max 30 chars. Real examples: 'BLUE MGTS-5109',
    -- 'PIG0000005-COM', 'CHM0000118'.
    CONSTRAINT chk_rm_group_code_format CHECK (
        group_code ~ '^[A-Z0-9][A-Z0-9 \-]{0,29}$'
    )
);

COMMENT ON TABLE cst_rm_group_head IS 'User-defined groups of raw materials sharing a landed-cost configuration.';

-- Active group_code must be unique (soft-deleted rows may reuse the code).
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_head_code_active
    ON cst_rm_group_head (group_code) WHERE deleted_at IS NULL;

-- Filter active groups quickly.
CREATE INDEX IF NOT EXISTS idx_rm_group_head_is_active
    ON cst_rm_group_head (is_active) WHERE deleted_at IS NULL;

-- Full-text search on code + name for UI pickers.
CREATE INDEX IF NOT EXISTS idx_rm_group_head_search
    ON cst_rm_group_head USING gin (to_tsvector('simple', group_code || ' ' || group_name));
