-- 000528 (backlog1 Track A) — TX Weight master (legacy MGTAPPS.CST_YARN_TX_WEIGHT).
--
-- Global (NOT per period) weight rule per product type x grade. Consumed by the
-- calc engine built-in tx_weight(grade, AX_WT, fallback) that F_YARN_{AE,A9,A,B,C}_WT
-- call after 000530:
--   LESS_BY  -> AX_WT - value
--   MULTIPLY -> AX_WT * value
--   FIXED    -> value
--   no row   -> fallback (the pre-000530 ratio AX_WT * G_PERC / AX_PERC)
--
-- ytw_product_type_id is INT (FK cost_product_type.cpt_type_id is SERIAL), not
-- UUID — matches the int32 product_type_id in finance/v1/yarn_tx_weight.proto.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_yarn_tx_weight (
    ytw_id              UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    ytw_product_type_id INT            NOT NULL REFERENCES cost_product_type (cpt_type_id),
    ytw_grade           VARCHAR(3)     NOT NULL,
    ytw_mode            VARCHAR(10)    NOT NULL,
    ytw_value           NUMERIC(18,6)  NOT NULL DEFAULT 0,
    ytw_description     VARCHAR(200),
    ytw_oracle_sys_id   VARCHAR(30),
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    created_by          VARCHAR(100)   NOT NULL,
    updated_at          TIMESTAMPTZ,
    updated_by          VARCHAR(100),
    deleted_at          TIMESTAMPTZ,
    deleted_by          VARCHAR(100),
    CONSTRAINT chk_mst_yarn_tx_weight_grade CHECK (ytw_grade IN ('AE', 'A9', 'A', 'B', 'C')),
    CONSTRAINT chk_mst_yarn_tx_weight_mode  CHECK (ytw_mode IN ('LESS_BY', 'MULTIPLY', 'FIXED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_yarn_tx_weight_type_grade
    ON mst_yarn_tx_weight (ytw_product_type_id, ytw_grade) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mst_yarn_tx_weight_type
    ON mst_yarn_tx_weight (ytw_product_type_id) WHERE deleted_at IS NULL;

COMMENT ON TABLE  mst_yarn_tx_weight IS 'TX Weight master (legacy CST_YARN_TX_WEIGHT): global per product type x grade weight rule used by the tx_weight() calc built-in.';
COMMENT ON COLUMN mst_yarn_tx_weight.ytw_mode IS 'LESS_BY: AX_WT - value; MULTIPLY: AX_WT * value; FIXED: value.';
COMMENT ON COLUMN mst_yarn_tx_weight.ytw_oracle_sys_id IS 'Legacy CYTW_SYS_ID (traceability only).';

COMMIT;
