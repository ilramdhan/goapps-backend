-- Migration: Create cst_item_cons_stk_po table.
-- Description: Item Consumption, Stock, and PO data synced from Oracle MGTDAT.MGT_ITEM_CONS_STK_PO.

CREATE TABLE IF NOT EXISTS cst_item_cons_stk_po (
    -- Primary key (composite, matches Oracle source).
    period          VARCHAR(6)   NOT NULL,
    item_code       VARCHAR(20)  NOT NULL,
    grade_code      VARCHAR(40)  NOT NULL,

    -- Descriptive fields.
    grade_name      VARCHAR(240),
    item_name       VARCHAR(240),
    uom             VARCHAR(12),

    -- Consumption.
    cons_qty        NUMERIC(20,6),
    cons_val        NUMERIC(20,6),
    cons_rate       NUMERIC(20,6),

    -- Stores.
    stores_qty      NUMERIC(20,6),
    stores_val      NUMERIC(20,6),
    stores_rate     NUMERIC(20,6),

    -- Department.
    dept_qty        NUMERIC(20,6),
    dept_val        NUMERIC(20,6),
    dept_rate       NUMERIC(20,6),

    -- Last PO 1.
    last_po_qty1    NUMERIC(20,6),
    last_po_val1    NUMERIC(20,6),
    last_po_rate1   NUMERIC(20,6),
    last_po_dt1     TIMESTAMPTZ,

    -- Last PO 2.
    last_po_qty2    NUMERIC(20,6),
    last_po_val2    NUMERIC(20,6),
    last_po_rate2   NUMERIC(20,6),
    last_po_dt2     TIMESTAMPTZ,

    -- Last PO 3.
    last_po_qty3    NUMERIC(20,6),
    last_po_val3    NUMERIC(20,6),
    last_po_rate3   NUMERIC(20,6),
    last_po_dt3     TIMESTAMPTZ,

    -- Sync metadata.
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_by_job   UUID REFERENCES job_execution(job_id),

    PRIMARY KEY (period, item_code, grade_code)
);

COMMENT ON TABLE cst_item_cons_stk_po IS 'Item consumption, stock, and PO data synced from Oracle.';

-- Query pattern indexes.
CREATE INDEX IF NOT EXISTS idx_cst_micsp_period ON cst_item_cons_stk_po(period);
CREATE INDEX IF NOT EXISTS idx_cst_micsp_item_code ON cst_item_cons_stk_po(item_code);
CREATE INDEX IF NOT EXISTS idx_cst_micsp_synced_at ON cst_item_cons_stk_po(synced_at DESC);

-- Full-text search on item name.
CREATE INDEX IF NOT EXISTS idx_cst_micsp_search ON cst_item_cons_stk_po
    USING gin(to_tsvector('english', coalesce(item_code, '') || ' ' || coalesce(item_name, '') || ' ' || coalesce(grade_name, '')));
