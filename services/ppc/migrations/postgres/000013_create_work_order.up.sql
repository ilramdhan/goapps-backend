-- Layer 3: work order (v1.2 — route + product-parameter driven) + child tables.
-- work_order snapshots cost_route_head (crh) version + spec/packing JSONB at approve.
-- wo_ref_wo_id self-refs for template/continuation. Child tables carry real FKs to
-- work_order (same DB). Soft refs (crh, demand, param) stay plain BIGINT/UUID.
CREATE TABLE IF NOT EXISTS work_order (
    wo_id                    BIGSERIAL     PRIMARY KEY,
    wo_no                    VARCHAR(30)   NOT NULL UNIQUE,
    wo_lot_no                VARCHAR(30)   NOT NULL UNIQUE,   -- PPC-generated; bobbin tracking
    wo_area                  CHAR(3)       NOT NULL,          -- TXT / SPG / TWT
    wo_machine_id            BIGINT        NOT NULL,
    wo_crh_head_id           BIGINT        NOT NULL,          -- snapshot cost_route_head (soft ref)
    wo_crh_version           INT           NOT NULL,          -- snapshot route version
    wo_plan_item_id          BIGINT        NOT NULL,
    wo_demand_id             BIGINT,                          -- customer/grade req flows from here
    wo_ref_wo_id             BIGINT        REFERENCES work_order(wo_id), -- duplicate/continuation
    wo_ref_type              VARCHAR(15),                     -- TEMPLATE / CONTINUATION
    wo_qty_target            DECIMAL(18,3) NOT NULL,
    wo_grade_requirement     VARCHAR(5),                      -- default demand, WO may override
    wo_deadline              DATE          NOT NULL,
    wo_prod_category         VARCHAR(15)   NOT NULL DEFAULT 'NORMAL', -- NORMAL/B_TO_B/APQ/TRIAL/SMALL_LOT
    wo_spec_snapshot         JSONB,                           -- den/fil/type/ply/shade/twist at approve
    wo_packing_snapshot      JSONB,                           -- box/paper tube/pallet (master + override)
    wo_revision_no           SMALLINT      NOT NULL DEFAULT 0,
    wo_revision_reason       TEXT,                            -- shown on WO face (e.g. "PINDAH MC 05")
    wo_status                VARCHAR(20)   NOT NULL DEFAULT 'DRAFT',
    wo_pc_approved_at        TIMESTAMPTZ,                     -- PC -> PM sequential
    wo_pc_approved_by        BIGINT,
    wo_pm_approved_at        TIMESTAMPTZ,
    wo_pm_approved_by        BIGINT,
    wo_auto_approve_disabled BOOLEAN       DEFAULT FALSE,     -- if true, no run without explicit PM
    wo_plan_change_flag      BOOLEAN       NOT NULL DEFAULT FALSE,
    wo_plan_change_note      TEXT,
    wo_created_by            BIGINT        NOT NULL,
    wo_created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    wo_updated_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wo_area CHECK (wo_area IN ('TXT', 'SPG', 'TWT'))
);

CREATE INDEX IF NOT EXISTS idx_wo_plan_item_id ON work_order (wo_plan_item_id);
CREATE INDEX IF NOT EXISTS idx_wo_machine_id ON work_order (wo_machine_id);
CREATE INDEX IF NOT EXISTS idx_wo_status ON work_order (wo_status);

-- Planned parameter = one row per param (not fixed columns). Source: product-parameter
-- master (mst_parameter, display_group='Machine'). Dual PPC/PC only for 8 params.
CREATE TABLE IF NOT EXISTS wo_parameter (
    wop_id            BIGSERIAL    PRIMARY KEY,
    wop_wo_id         BIGINT       NOT NULL REFERENCES work_order(wo_id),
    wop_param_id      UUID         NOT NULL,          -- FK mst_parameter (costing, soft ref)
    wop_value_ppc_num  DECIMAL(20,6),
    wop_value_ppc_text TEXT,
    wop_value_ppc_flag BOOLEAN,
    wop_value_pc_num   DECIMAL(20,6),                 -- filled by PC at approve; default = PPC
    wop_value_pc_text  TEXT,
    wop_value_pc_flag  BOOLEAN,
    wop_is_dual       BOOLEAN      DEFAULT FALSE,      -- true: speed/disc/nozzle1&2/bar/air/oil/opu/taper
    CONSTRAINT uq_wop_wo_param UNIQUE (wop_wo_id, wop_param_id)
);

CREATE INDEX IF NOT EXISTS idx_wop_wo_id ON wo_parameter (wop_wo_id);

-- Actual parameter (per date+shift+param). Also holds actual-only (Heater/DR/Steps ACY).
CREATE TABLE IF NOT EXISTS wo_execution (
    woe_id          BIGSERIAL    PRIMARY KEY,
    woe_wo_id       BIGINT       NOT NULL REFERENCES work_order(wo_id),
    woe_date        DATE         NOT NULL,
    woe_shift       CHAR(1)      NOT NULL,   -- 1 / 2 / 3
    woe_param_id    UUID         NOT NULL,   -- FK mst_parameter (soft ref)
    woe_value_num   DECIMAL(20,6),
    woe_value_text  TEXT,
    woe_value_flag  BOOLEAN,
    woe_input_by    BIGINT       NOT NULL,
    woe_input_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_woe_wo_date_shift_param UNIQUE (woe_wo_id, woe_date, woe_shift, woe_param_id)
);

CREATE INDEX IF NOT EXISTS idx_woe_wo_id ON wo_execution (woe_wo_id);

-- N rows per RM route component (cost_route_rm). type=PRODUCT => cross-product genealogy.
CREATE TABLE IF NOT EXISTS wo_rm_allocation (
    wra_id               BIGSERIAL     PRIMARY KEY,
    wra_wo_id            BIGINT        NOT NULL REFERENCES work_order(wo_id),
    wra_crm_rm_id        BIGINT        NOT NULL,   -- FK cost_route_rm (soft ref)
    wra_rm_type          VARCHAR(10),              -- PRODUCT / ITEM / GROUP
    wra_lot_no           VARCHAR(30)   NOT NULL,   -- actual lot chosen
    wra_rm_source        VARCHAR(10),              -- STORE / CAPTIVE / MIXED
    wra_fresh_box        VARCHAR(5),               -- Fresh / Box
    wra_shade_code       VARCHAR(30),
    wra_qty_allocated    DECIMAL(18,3) NOT NULL,
    wra_lot_picking_mode VARCHAR(10),              -- STRICT / FLEXIBLE / FIFO
    wra_notes            TEXT
);

CREATE INDEX IF NOT EXISTS idx_wra_wo_id ON wo_rm_allocation (wra_wo_id);

CREATE TABLE IF NOT EXISTS wo_plan_item_link (
    wpl_id               BIGSERIAL     PRIMARY KEY,
    wpl_wo_id            BIGINT        NOT NULL REFERENCES work_order(wo_id),
    wpl_plan_item_id     BIGINT        NOT NULL REFERENCES production_plan_item(ppi_id),
    wpl_qty_contribution DECIMAL(18,3),
    CONSTRAINT uq_wpl_wo_plan_item UNIQUE (wpl_wo_id, wpl_plan_item_id)
);

CREATE TABLE IF NOT EXISTS wo_actual_log (
    wal_id            BIGSERIAL     PRIMARY KEY,
    wal_wo_id         BIGINT        NOT NULL REFERENCES work_order(wo_id),
    wal_qty_before    DECIMAL(18,3),
    wal_qty_after     DECIMAL(18,3),
    wal_source_before VARCHAR(20),
    wal_source_after  VARCHAR(20),
    wal_reason        TEXT,
    wal_edited_by     BIGINT        NOT NULL,
    wal_edited_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wal_wo_id ON wo_actual_log (wal_wo_id);
