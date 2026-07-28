-- Layer 1: production demand. pd_cpm_product_sys_id and pd_customer_id are soft refs
-- to finance (separate DB, no FK). Self-ref pd_carry_from_id for carry-forward; FK to
-- sales_order_staging for Orion-pulled demands.
CREATE TABLE IF NOT EXISTS production_demand (
    pd_id                 BIGSERIAL     PRIMARY KEY,
    pd_type               VARCHAR(20)   NOT NULL,   -- CONTRACT / MTS / SAMPLE
    pd_sub_type           VARCHAR(20),              -- CF_EXPORT / NEW_EXPORT / LOCAL / INTERNAL
    pd_source             VARCHAR(20)   NOT NULL,   -- ORION_PULL / MANUAL / MTS_APPROVED / CARRY_FORWARD
    pd_carry_action       VARCHAR(20),              -- CARRY_AS_IS / SPLIT / DEFER / PARTIAL_CARRY
    pd_cpm_product_sys_id BIGINT        NOT NULL,   -- soft ref to finance CPM
    pd_qty_original       DECIMAL(18,3) NOT NULL,
    pd_qty_remaining      DECIMAL(18,3) NOT NULL,
    pd_deadline           DATE          NOT NULL,
    pd_customer_id        BIGINT,                   -- soft ref
    pd_contract_no        VARCHAR(50),
    pd_contract_date      DATE,
    pd_stuff_advance_no   VARCHAR(50),
    pd_incoterm           VARCHAR(10),
    pd_lc_status          VARCHAR(30),
    pd_grade_requirement  VARCHAR(20),              -- AX_ONLY / AX_AM_CLAUSE / NONE
    pd_ax_min_pct         DECIMAL(5,2),
    pd_am_max_pct         DECIMAL(5,2),
    pd_carry_from_id      BIGINT        REFERENCES production_demand(pd_id),
    pd_sos_ref            BIGINT        REFERENCES sales_order_staging(sos_id),
    pd_status             VARCHAR(30)   NOT NULL,
    pd_month              CHAR(7)       NOT NULL,   -- YYYY-MM
    pd_confirmed_by       BIGINT,
    pd_confirmed_at       TIMESTAMPTZ,
    pd_created_by         BIGINT        NOT NULL,
    pd_created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    pd_updated_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_pd_type CHECK (pd_type IN ('CONTRACT', 'MTS', 'SAMPLE'))
);

CREATE INDEX IF NOT EXISTS idx_pd_month_status ON production_demand (pd_month, pd_status);
