-- Migration: Create cst_rm_cost — the result table for per-period RM landed cost.
-- UPSERTed (period, rm_code) by the calculation worker.

CREATE TABLE IF NOT EXISTS cst_rm_cost (
    rm_cost_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity of the cost row.
    period                VARCHAR(6)  NOT NULL,                -- YYYYMM
    rm_code               VARCHAR(30) NOT NULL,                -- group_code (phase 1) or item_code (phase 2)
    rm_type               VARCHAR(20) NOT NULL,                -- 'GROUP' or 'ITEM'
    group_head_id         UUID REFERENCES cst_rm_group_head(group_head_id) ON DELETE SET NULL,
    item_code             VARCHAR(20),                         -- NULL when rm_type='GROUP'
    rm_name               VARCHAR(200),
    uom_code              VARCHAR(12),

    -- Aggregated per-stage rates (SUM(val)/SUM(qty) across the group's active items).
    cons_rate             DECIMAL(20,6),
    stores_rate           DECIMAL(20,6),
    dept_rate             DECIMAL(20,6),
    po_rate_1             DECIMAL(20,6),
    po_rate_2             DECIMAL(20,6),
    po_rate_3             DECIMAL(20,6),

    -- Landed costs per purpose (raw value; UI layer formats for display).
    cost_val              DECIMAL(20,6),
    cost_mark             DECIMAL(20,6),
    cost_sim              DECIMAL(20,6),

    -- Snapshot of flags as configured on the header at calculation time.
    flag_valuation        VARCHAR(20) NOT NULL,
    flag_marketing        VARCHAR(20) NOT NULL,
    flag_simulation       VARCHAR(20) NOT NULL,

    -- Flag actually used after cascade resolution (differs from requested when cascade triggered).
    flag_valuation_used   VARCHAR(20) NOT NULL,
    flag_marketing_used   VARCHAR(20) NOT NULL,
    flag_simulation_used  VARCHAR(20) NOT NULL,

    -- Traceability.
    calculated_at         TIMESTAMPTZ,
    calculated_by         VARCHAR(100),

    -- Audit.
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by            VARCHAR(100) NOT NULL,
    updated_at            TIMESTAMPTZ,
    updated_by            VARCHAR(100),

    CONSTRAINT chk_rm_cost_rm_type CHECK (rm_type IN ('GROUP','ITEM')),
    CONSTRAINT chk_rm_cost_period_format CHECK (period ~ '^[0-9]{6}$'),
    CONSTRAINT chk_rm_cost_flag_valuation  CHECK (flag_valuation  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_cost_flag_marketing  CHECK (flag_marketing  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_cost_flag_simulation CHECK (flag_simulation IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_cost_flag_valuation_used  CHECK (flag_valuation_used  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_cost_flag_marketing_used  CHECK (flag_marketing_used  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_cost_flag_simulation_used CHECK (flag_simulation_used IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT'))
);

COMMENT ON TABLE cst_rm_cost IS 'Per-period landed cost per RM (group in phase 1). UPSERTed by calculation worker on (period, rm_code).';

-- UPSERT target.
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_cost_period_rm
    ON cst_rm_cost (period, rm_code);

-- Period filtering (most listing queries filter by period).
CREATE INDEX IF NOT EXISTS idx_rm_cost_period
    ON cst_rm_cost (period);

-- Lookup cost history by group.
CREATE INDEX IF NOT EXISTS idx_rm_cost_group_head
    ON cst_rm_cost (group_head_id) WHERE group_head_id IS NOT NULL;

-- Most-recently-calculated listing.
CREATE INDEX IF NOT EXISTS idx_rm_cost_calculated_at
    ON cst_rm_cost (calculated_at DESC);
