-- Layer 2: production plan item + change log. ppi_cpm_product_sys_id is a soft ref to
-- finance CPM. CHECK enforces every item ties to a demand OR a parent item (cascade
-- intermediate). production_plan_log records field-level change history.
CREATE TABLE IF NOT EXISTS production_plan_item (
    ppi_id                   BIGSERIAL     PRIMARY KEY,
    ppi_cpm_product_sys_id   BIGINT        NOT NULL,   -- soft ref to finance CPM
    ppi_type                 VARCHAR(20)   NOT NULL,   -- FG_DELIVERY / INTERMEDIATE / MTS
    ppi_demand_id            BIGINT        REFERENCES production_demand(pd_id),
    ppi_parent_item_id       BIGINT        REFERENCES production_plan_item(ppi_id),
    ppi_qty_target           DECIMAL(18,3) NOT NULL,
    ppi_deadline             DATE          NOT NULL,
    ppi_rm_source            VARCHAR(10),              -- STORE / CAPTIVE / MIXED
    ppi_sequence             INT           NOT NULL DEFAULT 0,
    ppi_status               VARCHAR(20)   NOT NULL,
    ppi_machine_group_id     BIGINT        NOT NULL,
    ppi_preferred_machine_id BIGINT,
    ppi_month                CHAR(7)       NOT NULL,   -- YYYY-MM
    ppi_notes                TEXT,
    ppi_created_by           BIGINT        NOT NULL,
    ppi_created_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    ppi_updated_at           TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ppi_demand_or_parent CHECK (ppi_demand_id IS NOT NULL OR ppi_parent_item_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_ppi_demand_id ON production_plan_item (ppi_demand_id);
CREATE INDEX IF NOT EXISTS idx_ppi_month ON production_plan_item (ppi_month, ppi_status);

CREATE TABLE IF NOT EXISTS production_plan_log (
    ppl_id           BIGSERIAL     PRIMARY KEY,
    ppl_plan_item_id BIGINT        NOT NULL REFERENCES production_plan_item(ppi_id),
    ppl_field_changed VARCHAR(50)  NOT NULL,
    ppl_value_before TEXT,
    ppl_value_after  TEXT,
    ppl_changed_by   BIGINT        NOT NULL,
    ppl_changed_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    ppl_reason       TEXT
);

CREATE INDEX IF NOT EXISTS idx_ppl_plan_item_id ON production_plan_log (ppl_plan_item_id);
