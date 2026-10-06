-- 000549 — ERP integration tables (design Part 1 §4.2; plan-01 P0-T3).
--
--   * cst_erp_int_batch  : the monthly batch aggregate root. ceib_batch_id is
--                          pushed to Oracle as GSB_BATCH_ID / FLEX_13.
--   * cst_erp_adj_demand : ADJ demand snapshot per batch, replaced on reload.
--   * cst_erp_coverage   : (item, shade) -> GoApps AX cost coverage (C-5).
--   * cst_erp_std_cost   : derived std rows, the source of GSC_* columns.
--
-- Batch status (design §5.3, 11 values). Active = VALUATED/RECONCILED/LOCKED
-- (C-8); in-flight = DRAFT..PUSHED. Partial unique indexes allow at most one
-- active and one in-flight LIVE batch per period. SHADOW batches (historical
-- backtest, PG only; User decision 2026-09-29 U-4) are excluded from both and
-- can never reach PUSHED or beyond (chk_ceib_shadow_status).
--
-- New tables only; nothing existing changes.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_erp_int_batch (
    ceib_batch_id           BIGSERIAL     PRIMARY KEY,
    ceib_period             VARCHAR(6)    NOT NULL,
    ceib_seq                INT           NOT NULL,
    ceib_status             VARCHAR(16)   NOT NULL DEFAULT 'DRAFT',
    ceib_job_id             UUID          NULL,
    ceib_demand_loaded_at   TIMESTAMPTZ   NULL,
    ceib_rule_snapshot      JSONB         NULL,
    ceib_rule_hash          CHAR(64)      NULL,
    ceib_row_count          INT           NULL,
    ceib_sum_std            NUMERIC(24,5) NULL,
    ceib_sum_conv           NUMERIC(24,5) NULL,
    ceib_sum_pvl            NUMERIC(24,5) NULL,
    ceib_summary            JSONB         NOT NULL DEFAULT '{}'::jsonb,
    ceib_valuation_progress JSONB         NOT NULL DEFAULT '{}'::jsonb,
    ceib_warnings_ack_by    VARCHAR(64)   NULL,
    ceib_warnings_ack_at    TIMESTAMPTZ   NULL,
    ceib_pushed_at          TIMESTAMPTZ   NULL,
    ceib_pushed_by          VARCHAR(64)   NULL,
    ceib_valuated_at        TIMESTAMPTZ   NULL,
    ceib_valuated_by        VARCHAR(64)   NULL,
    ceib_reconciled_at      TIMESTAMPTZ   NULL,
    ceib_adj_approved_at    TIMESTAMPTZ   NULL,
    ceib_adj_approved_by    VARCHAR(64)   NULL,
    ceib_locked_at          TIMESTAMPTZ   NULL,
    ceib_locked_by          VARCHAR(64)   NULL,
    ceib_needs_repush       BOOLEAN       NOT NULL DEFAULT FALSE,
    ceib_error              TEXT          NULL,
    ceib_mode               VARCHAR(8)    NOT NULL DEFAULT 'LIVE',
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by              VARCHAR(64)   NOT NULL,
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_by              VARCHAR(64)   NULL,
    CONSTRAINT chk_ceib_period CHECK (ceib_period ~ '^[0-9]{4}(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_ceib_seq    CHECK (ceib_seq >= 1),
    CONSTRAINT chk_ceib_status CHECK (ceib_status IN (
        'DRAFT', 'DEMAND_LOADED', 'COVERED', 'DERIVED', 'VALIDATED', 'PUSHED',
        'VALUATED', 'RECONCILED', 'LOCKED', 'FAILED', 'SUPERSEDED')),
    CONSTRAINT chk_ceib_mode CHECK (ceib_mode IN ('LIVE', 'SHADOW')),
    CONSTRAINT chk_ceib_shadow_status CHECK (
        ceib_mode = 'LIVE'
        OR ceib_status IN ('DRAFT', 'DEMAND_LOADED', 'COVERED', 'DERIVED', 'VALIDATED', 'FAILED')),
    CONSTRAINT uk_ceib_period_seq UNIQUE (ceib_period, ceib_seq)
);

COMMENT ON TABLE cst_erp_int_batch IS
    'ERP cost integration batch per period (design §4.2, §5.3). ceib_batch_id = Oracle GSB_BATCH_ID = FLEX_13.';

CREATE UNIQUE INDEX IF NOT EXISTS uix_ceib_active
    ON cst_erp_int_batch (ceib_period)
    WHERE ceib_status IN ('VALUATED', 'RECONCILED', 'LOCKED') AND ceib_mode = 'LIVE';

CREATE UNIQUE INDEX IF NOT EXISTS uix_ceib_inflight
    ON cst_erp_int_batch (ceib_period)
    WHERE ceib_status IN ('DRAFT', 'DEMAND_LOADED', 'COVERED', 'DERIVED', 'VALIDATED', 'PUSHED')
      AND ceib_mode = 'LIVE';

CREATE INDEX IF NOT EXISTS idx_ceib_status
    ON cst_erp_int_batch (ceib_status);

CREATE TABLE IF NOT EXISTS cst_erp_adj_demand (
    ced_id             BIGSERIAL     PRIMARY KEY,
    ced_batch_id       BIGINT        NOT NULL
        REFERENCES cst_erp_int_batch (ceib_batch_id) ON DELETE CASCADE,
    ced_period         VARCHAR(6)    NOT NULL,
    ced_txn_code       VARCHAR(12)   NOT NULL,
    ced_item_kind      VARCHAR(4)    NOT NULL,
    ced_item_code      VARCHAR(50)   NOT NULL,
    ced_item_name      VARCHAR(500)  NULL,
    ced_grade_code     VARCHAR(20)   NOT NULL,
    ced_shade_code     VARCHAR(20)   NOT NULL,
    ced_item_count     INT           NOT NULL,
    ced_rate_variants  INT           NOT NULL,
    ced_qty_kg         NUMERIC(24,7) NOT NULL,
    ced_min_rate       NUMERIC(20,7) NULL,
    ced_max_rate       NUMERIC(20,7) NULL,
    ced_adj_val        NUMERIC(24,7) NULL,
    ced_approved_items INT           NOT NULL,
    ced_posted_items   INT           NOT NULL,
    ced_goapps_batch   VARCHAR(20)   NULL,
    ced_goapps_source  VARCHAR(16)   NULL,
    ced_loaded_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ced_item_kind CHECK (ced_item_kind IN ('YARN', 'MB')),
    CONSTRAINT uk_ced UNIQUE (ced_batch_id, ced_txn_code, ced_item_code, ced_grade_code, ced_shade_code)
);

COMMENT ON TABLE cst_erp_adj_demand IS
    'ADJ demand snapshot per ERP batch (design §4.2), read-only from Oracle; replaced on each reload.';

CREATE INDEX IF NOT EXISTS idx_ced_item_shade
    ON cst_erp_adj_demand (ced_period, ced_item_code, ced_shade_code);

CREATE TABLE IF NOT EXISTS cst_erp_coverage (
    cec_id             BIGSERIAL     PRIMARY KEY,
    cec_batch_id       BIGINT        NOT NULL
        REFERENCES cst_erp_int_batch (ceib_batch_id) ON DELETE CASCADE,
    cec_item_kind      VARCHAR(4)    NOT NULL,
    cec_item_code      VARCHAR(50)   NOT NULL,
    cec_shade_code     VARCHAR(20)   NOT NULL,
    cec_grade_codes    TEXT[]        NOT NULL,
    cec_product_sys_id BIGINT        NULL,
    cec_cost_id        BIGINT        NULL,
    cec_cost_version   INT           NULL,
    cec_status         VARCHAR(16)   NOT NULL,
    cec_reason         TEXT          NULL,
    cec_qty_kg         NUMERIC(24,7) NOT NULL,
    cec_candidates     JSONB         NULL,
    CONSTRAINT chk_cec_status CHECK (cec_status IN (
        'OK', 'NO_MAPPING', 'DUP_MAPPING', 'NO_COST', 'NOT_APPROVED', 'NOT_USD', 'INVALID')),
    CONSTRAINT uk_cec UNIQUE (cec_batch_id, cec_item_code, cec_shade_code)
);

COMMENT ON TABLE cst_erp_coverage IS
    'Coverage of ERP (item, shade) demand by GoApps AX cost per batch (design §4.2, C-5). cec_cost_id -> cst_product_cost.cpc_cost_id (X-6).';

CREATE INDEX IF NOT EXISTS idx_cec_status
    ON cst_erp_coverage (cec_batch_id, cec_status);

CREATE TABLE IF NOT EXISTS cst_erp_std_cost (
    cesc_id                BIGSERIAL     PRIMARY KEY,
    cesc_batch_id          BIGINT        NOT NULL
        REFERENCES cst_erp_int_batch (ceib_batch_id) ON DELETE CASCADE,
    cesc_period            VARCHAR(6)    NOT NULL,
    cesc_item_code         VARCHAR(50)   NOT NULL,
    cesc_grade_code        VARCHAR(20)   NOT NULL,
    cesc_shade_code        VARCHAR(20)   NOT NULL,
    cesc_item_name         VARCHAR(240)  NULL,
    cesc_shade_name        VARCHAR(240)  NULL,
    cesc_item_kind         VARCHAR(4)    NOT NULL,
    cesc_source            VARCHAR(16)   NULL,
    cesc_ax_cost_sys_id    BIGINT        NULL,
    cesc_ax_cost_version   INT           NULL,
    cesc_product_sys_id    BIGINT        NULL,
    cesc_prod_type         VARCHAR(3)    NULL,
    cesc_grade_group       VARCHAR(20)   NULL,
    cesc_fg_type           VARCHAR(15)   NULL,
    cesc_basis             VARCHAR(15)   NULL,
    cesc_chp_item_code     VARCHAR(50)   NULL,
    cesc_chp_con_kg        NUMERIC(20,5) NULL,
    cesc_chp_cost          NUMERIC(20,5) NULL,
    cesc_ax_conv_cost      NUMERIC(20,5) NULL,
    cesc_conv_cost         NUMERIC(20,5) NULL,
    cesc_conv_cost1        NUMERIC(20,5) NULL,
    cesc_conv_cost2        NUMERIC(20,5) NULL,
    cesc_conv_cost4        NUMERIC(20,5) NULL,
    cesc_conv_cost5        NUMERIC(20,5) NULL,
    cesc_selling_price     NUMERIC(20,5) NULL,
    cesc_value_loss        NUMERIC(20,5) NULL,
    cesc_ax_cost           NUMERIC(20,5) NULL,
    cesc_std_cost          NUMERIC(20,5) NULL,
    cesc_prod_value_loss   NUMERIC(20,5) NULL,
    cesc_ms_batch_item     VARCHAR(12)   NULL,
    cesc_item_type         VARCHAR(20)   NULL,
    cesc_prd_per_day       NUMERIC(20,5) NULL,
    cesc_status            VARCHAR(16)   NOT NULL,
    cesc_validation        JSONB         NOT NULL DEFAULT '[]'::jsonb,
    cesc_prev_std_cost     NUMERIC(20,5) NULL,
    cesc_erp_rate          NUMERIC(20,7) NULL,
    cesc_erp_rate_variants INT           NULL,
    cesc_erp_qty_kg        NUMERIC(24,7) NULL,
    cesc_erp_value         NUMERIC(24,7) NULL,
    cesc_erp_flex13        VARCHAR(20)   NULL,
    cesc_recon_status      VARCHAR(12)   NULL,
    CONSTRAINT chk_cesc_source CHECK (cesc_source IS NULL OR cesc_source IN ('GOAPPS_AX', 'GOAPPS_DERIVED', 'GOAPPS_MB')),
    CONSTRAINT chk_cesc_status CHECK (cesc_status IN (
        'OK', 'NO_AX', 'NO_RULE', 'NO_FG_TYPE', 'NO_GRADE_GROUP', 'NO_SELL_PRICE', 'INVALID')),
    -- C-11: an OK row is pushed to Oracle, whose key columns are VARCHAR2(12).
    CONSTRAINT chk_cesc_ok_key_len CHECK (
        cesc_status <> 'OK'
        OR (length(cesc_item_code) <= 12 AND length(cesc_grade_code) <= 12 AND length(cesc_shade_code) <= 12)),
    CONSTRAINT chk_cesc_recon_status CHECK (cesc_recon_status IS NULL OR cesc_recon_status IN ('MATCH', 'DIFF', 'NOT_IN_ADJ')),
    CONSTRAINT uk_cesc UNIQUE (cesc_batch_id, cesc_item_code, cesc_grade_code, cesc_shade_code)
);

COMMENT ON TABLE cst_erp_std_cost IS
    'Derived ERP std cost per (item, grade, shade) per batch (design §4.2); source of Oracle GSC_* columns. 5 dp by construction.';

CREATE INDEX IF NOT EXISTS idx_cesc_period_key
    ON cst_erp_std_cost (cesc_period, cesc_item_code, cesc_shade_code);

CREATE INDEX IF NOT EXISTS idx_cesc_status
    ON cst_erp_std_cost (cesc_batch_id, cesc_status);

COMMIT;
