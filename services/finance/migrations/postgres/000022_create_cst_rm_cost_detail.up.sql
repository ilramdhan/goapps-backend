-- Migration: V2 RM Cost engine — per-(cost, item, grade) snapshot table.
-- Mirrors the Excel "RM Cost Detail" rows (columns A–AR in the reference).

CREATE TABLE IF NOT EXISTS cst_rm_cost_detail (
    cost_detail_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rm_cost_id      UUID NOT NULL REFERENCES cst_rm_cost(rm_cost_id) ON DELETE CASCADE,
    period          VARCHAR(6)  NOT NULL,
    group_head_id   UUID        NOT NULL REFERENCES cst_rm_group_head(group_head_id) ON DELETE CASCADE,
    group_detail_id UUID,                        -- snapshot link, nullable if detail later deleted
    item_code       VARCHAR(50) NOT NULL,
    item_name       VARCHAR(200),
    grade_code      VARCHAR(40),

    -- Per-detail inputs (snapshot from cst_rm_group_detail at calc time).
    freight_rate            DECIMAL(20,8),
    anti_dumping_pct        DECIMAL(20,8),       -- decimal (0.10 = 10%)
    duty_pct                DECIMAL(20,8),       -- decimal
    transport_rate          DECIMAL(20,8),
    valuation_default_value DECIMAL(20,8),

    -- Consumption stage.
    cons_val               DECIMAL(20,8),
    cons_qty               DECIMAL(20,8),
    cons_rate              DECIMAL(20,8),
    cons_freight_val       DECIMAL(20,8),
    cons_val_based         DECIMAL(20,8),
    cons_rate_based        DECIMAL(20,8),
    cons_anti_dumping_val  DECIMAL(20,8),
    cons_anti_dumping_rate DECIMAL(20,8),
    cons_duty_val          DECIMAL(20,8),
    cons_duty_rate         DECIMAL(20,8),
    cons_transport_val     DECIMAL(20,8),
    cons_transport_rate    DECIMAL(20,8),
    cons_landed_cost       DECIMAL(20,8),

    -- Stock stage.
    stock_val               DECIMAL(20,8),
    stock_qty               DECIMAL(20,8),
    stock_rate              DECIMAL(20,8),
    stock_freight_val       DECIMAL(20,8),
    stock_val_based         DECIMAL(20,8),
    stock_rate_based        DECIMAL(20,8),
    stock_anti_dumping_val  DECIMAL(20,8),
    stock_anti_dumping_rate DECIMAL(20,8),
    stock_duty_val          DECIMAL(20,8),
    stock_duty_rate         DECIMAL(20,8),
    stock_transport_val     DECIMAL(20,8),
    stock_transport_rate    DECIMAL(20,8),
    stock_landed_cost       DECIMAL(20,8),

    -- PO stage.
    po_val   DECIMAL(20,8),
    po_qty   DECIMAL(20,8),
    po_rate  DECIMAL(20,8),

    -- Fix stage. fix_rate is editable post-calc; the rest derive from it.
    fix_rate              DECIMAL(20,8),
    fix_freight_rate      DECIMAL(20,8),
    fix_rate_based        DECIMAL(20,8),
    fix_anti_dumping_rate DECIMAL(20,8),
    fix_duty_rate         DECIMAL(20,8),
    fix_transport_rate    DECIMAL(20,8),
    fix_landed_cost       DECIMAL(20,8),

    -- Audit.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL DEFAULT 'system',
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),

    CONSTRAINT chk_rm_cost_detail_period_format CHECK (period ~ '^[0-9]{6}$')
);

COMMENT ON TABLE cst_rm_cost_detail IS 'V2: Per-item per-grade snapshot of intermediate columns produced by the RM cost calculation engine. Mirrors Excel rows 9–11 in the reference workbook.';

CREATE INDEX IF NOT EXISTS idx_rm_cost_detail_cost_id ON cst_rm_cost_detail (rm_cost_id);
CREATE INDEX IF NOT EXISTS idx_rm_cost_detail_period_item ON cst_rm_cost_detail (period, item_code, grade_code);
CREATE INDEX IF NOT EXISTS idx_rm_cost_detail_group_head ON cst_rm_cost_detail (group_head_id);
