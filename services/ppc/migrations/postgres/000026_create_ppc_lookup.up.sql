-- PPC lookup master. Lightweight, PPC-owned DISPLAY-metadata table that gives the
-- frontend admin-configurable dropdown labels/order/active-state WITHOUT making
-- business enums dynamic. Backend business logic keeps validating against Go enum
-- constants; codes in this table MUST equal those enum string values. Do NOT
-- couple to finance's heavier mst_lookup, and do NOT route business decisions
-- through this table.
CREATE TABLE IF NOT EXISTS ppc_lookup (
    pl_id         BIGSERIAL     PRIMARY KEY,
    pl_category   VARCHAR(40)   NOT NULL,
    pl_code       VARCHAR(40)   NOT NULL,
    pl_label      VARCHAR(120)  NOT NULL,
    pl_sort_order INT           NOT NULL DEFAULT 0,
    pl_is_active  BOOLEAN       NOT NULL DEFAULT TRUE,
    pl_created_at TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    pl_created_by VARCHAR(100)  NOT NULL DEFAULT 'system',
    pl_updated_at TIMESTAMPTZ,
    pl_updated_by VARCHAR(100),
    CONSTRAINT uq_ppc_lookup_category_code UNIQUE (pl_category, pl_code)
);

CREATE INDEX IF NOT EXISTS idx_ppc_lookup_category ON ppc_lookup (pl_category, pl_sort_order);
