-- 000559 — ERP backtest report lines (plan-06 P5-T10b; design Part 2 §10.1, U-4).
--
-- One row per compared key of a SHADOW backtest batch (MATCH / DIFF /
-- ONLY_GOAPPS / ONLY_LEGACY). Replaced wholesale on every backtest run of the
-- batch. The report Failed flag and the class counts are derived from the lines.
--
-- New table only; nothing existing changes.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_erp_backtest_line (
    cebl_id         BIGSERIAL    PRIMARY KEY,
    cebl_batch_id   BIGINT       NOT NULL REFERENCES cst_erp_int_batch (ceib_batch_id) ON DELETE CASCADE,
    cebl_period     VARCHAR(6)   NOT NULL,
    cebl_item_code  VARCHAR(64)  NOT NULL,
    cebl_grade_code VARCHAR(64)  NOT NULL DEFAULT '',
    cebl_shade_code VARCHAR(64)  NOT NULL DEFAULT '',
    cebl_class      VARCHAR(16)  NOT NULL,
    cebl_basis      VARCHAR(16)  NOT NULL DEFAULT '',
    cebl_goapps     NUMERIC      NULL,
    cebl_legacy     NUMERIC      NULL,
    cebl_delta      NUMERIC      NULL,
    cebl_delta_pct  NUMERIC      NULL,
    cebl_fail       BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_cebl_period CHECK (cebl_period ~ '^[0-9]{4}(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_cebl_class CHECK (cebl_class IN ('MATCH', 'DIFF', 'ONLY_GOAPPS', 'ONLY_LEGACY'))
);

CREATE INDEX IF NOT EXISTS idx_cebl_batch ON cst_erp_backtest_line (cebl_batch_id);

COMMENT ON TABLE cst_erp_backtest_line IS
    'ERP backtest comparison lines of a SHADOW batch vs legacy ADJ rates (design Part 2 §10.1, U-4).';

COMMIT;
