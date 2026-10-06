-- 000554 — Oracle safety tables (design Part 1 §3.5 G7, §4.7, §S-R8/§S-R11;
-- plan-01 P0-T5).
--
--   * cst_erp_valuation_preview : the mandatory preview behind every ADJ-
--     mutating call (VALUATE/APPROVE/RESTORE). OPEN -> CONSUMED exactly once;
--     TTL via cevp_expires_at; at most one live (BUILDING/OPEN) preview per
--     (batch, operation).
--   * cst_erp_adj_snapshot      : immutable PG backup of every affected
--     OT_ADJ_ITEM row, captured before any call (G7). A BEFORE UPDATE OR
--     DELETE trigger raises, so rows can only be inserted. Retention: forever.
--   * cst_erp_oracle_call_log   : one row per Oracle write attempt, keyed by
--     the 6-key allowlist (design §3.3). ceocl_params holds non-secret binds
--     only (ids, counts, hash) — never credentials.
--
-- Status vocabularies follow design §4.7 (authoritative over plan-01 T5
-- wording): preview BUILDING/OPEN/CONSUMED/EXPIRED/STALE/FAILED; call log
-- STARTED/SUCCESS/FAILED/UNKNOWN. The idempotent re-run outcome ALREADY_DONE
-- (G9) is recorded in the batch summary, not as a call-log status, because no
-- Oracle call is made.
--
-- New tables only. PG side only: nothing here talks to Oracle.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_erp_valuation_preview (
    cevp_preview_id  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    cevp_batch_id    BIGINT      NOT NULL
        REFERENCES cst_erp_int_batch (ceib_batch_id),
    cevp_operation   VARCHAR(10) NOT NULL,
    cevp_status      VARCHAR(10) NOT NULL DEFAULT 'BUILDING',
    cevp_head_count  INT         NULL,
    cevp_item_count  INT         NULL,
    cevp_excluded    JSONB       NULL,
    cevp_set_hash    CHAR(64)    NULL,
    cevp_totals      JSONB       NULL,
    cevp_job_id      UUID        NULL,
    cevp_confirm_text TEXT       NULL,
    cevp_created_by  VARCHAR(64) NOT NULL,
    cevp_created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cevp_expires_at  TIMESTAMPTZ NOT NULL,
    cevp_consumed_at TIMESTAMPTZ NULL,
    cevp_consumed_by VARCHAR(64) NULL,
    CONSTRAINT chk_cevp_operation CHECK (cevp_operation IN ('VALUATE', 'APPROVE', 'RESTORE')),
    CONSTRAINT chk_cevp_status    CHECK (cevp_status IN ('BUILDING', 'OPEN', 'CONSUMED', 'EXPIRED', 'STALE', 'FAILED')),
    CONSTRAINT chk_cevp_expiry    CHECK (cevp_expires_at > cevp_created_at),
    CONSTRAINT chk_cevp_consumed  CHECK (cevp_status <> 'CONSUMED' OR (cevp_consumed_at IS NOT NULL AND cevp_consumed_by IS NOT NULL))
);

COMMENT ON TABLE cst_erp_valuation_preview IS
    'Mandatory preview token for ADJ-mutating ERP calls (design §3.2, §4.7). OPEN -> CONSUMED exactly once; TTL 30 min by default.';
COMMENT ON COLUMN cst_erp_valuation_preview.cevp_confirm_text IS
    'The typed confirmation text the operator must enter to execute (§3.2 G5).';

CREATE INDEX IF NOT EXISTS idx_cevp_batch_status
    ON cst_erp_valuation_preview (cevp_batch_id, cevp_status);

CREATE UNIQUE INDEX IF NOT EXISTS uix_cevp_live
    ON cst_erp_valuation_preview (cevp_batch_id, cevp_operation)
    WHERE cevp_status IN ('BUILDING', 'OPEN');

CREATE TABLE IF NOT EXISTS cst_erp_adj_snapshot (
    ceas_id               BIGSERIAL     PRIMARY KEY,
    ceas_preview_id       UUID          NOT NULL
        REFERENCES cst_erp_valuation_preview (cevp_preview_id),
    ceas_batch_id         BIGINT        NOT NULL,
    ceas_period           VARCHAR(6)    NOT NULL,
    ceas_adjh_sys_id      BIGINT        NOT NULL,
    ceas_adji_sys_id      BIGINT        NOT NULL,
    ceas_txn_code         VARCHAR(12)   NOT NULL,
    ceas_head_appr_status INT           NULL,
    ceas_head_post_status VARCHAR(20)   NULL,
    ceas_item_code        VARCHAR(50)   NULL,
    ceas_grade_code       VARCHAR(20)   NULL,
    ceas_shade_code       VARCHAR(20)   NULL,
    ceas_qty_bu           NUMERIC(24,7) NULL,
    ceas_item_desc        VARCHAR(500)  NULL,
    ceas_rate             NUMERIC(24,7) NULL,
    ceas_val              NUMERIC(24,7) NULL,
    ceas_flex             JSONB         NOT NULL,
    ceas_new_rate         NUMERIC(24,7) NULL,
    ceas_new_val          NUMERIC(24,7) NULL,
    ceas_new_flex         JSONB         NULL,
    ceas_row_hash         CHAR(64)      NOT NULL,
    ceas_captured_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_ceas UNIQUE (ceas_preview_id, ceas_adji_sys_id)
);

COMMENT ON TABLE cst_erp_adj_snapshot IS
    'Immutable PG backup of OT_ADJ_ITEM rows captured before any ADJ-mutating call (design §3.5 G7, §4.7). Insert-only (trg_ceas_immutable). Retention: forever.';

CREATE INDEX IF NOT EXISTS idx_ceas_batch_head
    ON cst_erp_adj_snapshot (ceas_batch_id, ceas_adjh_sys_id);

CREATE OR REPLACE FUNCTION cst_erp_adj_snapshot_forbid_mutation()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'cst_erp_adj_snapshot is immutable — % not permitted', TG_OP;
END;
$$;

DROP TRIGGER IF EXISTS trg_ceas_immutable ON cst_erp_adj_snapshot;

CREATE TRIGGER trg_ceas_immutable
    BEFORE UPDATE OR DELETE ON cst_erp_adj_snapshot
    FOR EACH ROW EXECUTE FUNCTION cst_erp_adj_snapshot_forbid_mutation();

CREATE TABLE IF NOT EXISTS cst_erp_oracle_call_log (
    ceocl_id            BIGSERIAL   PRIMARY KEY,
    ceocl_call_id       UUID        NOT NULL,
    ceocl_batch_id      BIGINT      NULL,
    ceocl_preview_id    UUID        NULL,
    ceocl_job_id        UUID        NULL,
    ceocl_statement_key VARCHAR(24) NOT NULL,
    ceocl_adjh_sys_id   BIGINT      NULL,
    ceocl_params        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ceocl_status        VARCHAR(10) NOT NULL,
    ceocl_attempt       SMALLINT    NOT NULL DEFAULT 1,
    ceocl_ora_code      VARCHAR(12) NULL,
    ceocl_error         TEXT        NULL,
    ceocl_summary       JSONB       NULL,
    ceocl_rows          INT         NULL,
    ceocl_actor         VARCHAR(64) NOT NULL,
    ceocl_request_id    VARCHAR(64) NULL,
    ceocl_started_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ceocl_finished_at   TIMESTAMPTZ NULL,
    CONSTRAINT uk_ceocl_call_id UNIQUE (ceocl_call_id),
    CONSTRAINT chk_ceocl_statement_key CHECK (ceocl_statement_key IN (
        'W1_INSERT_BATCH', 'W1_INSERT_COST',
        'W2_VALUATE_ADJ', 'W2_APPROVE_ADJ', 'W2_RESTORE_ADJ', 'W2_LOCK_BATCH')),
    CONSTRAINT chk_ceocl_status  CHECK (ceocl_status IN ('STARTED', 'SUCCESS', 'FAILED', 'UNKNOWN')),
    CONSTRAINT chk_ceocl_attempt CHECK (ceocl_attempt >= 1)
);

COMMENT ON TABLE cst_erp_oracle_call_log IS
    'One row per Oracle write attempt via the 6-key allowlist (design §3.3, §S-R11). Params are non-secret binds only. STARTED rows become UNKNOWN on worker start.';

CREATE INDEX IF NOT EXISTS idx_ceocl_batch
    ON cst_erp_oracle_call_log (ceocl_batch_id, ceocl_started_at);

CREATE INDEX IF NOT EXISTS idx_ceocl_status
    ON cst_erp_oracle_call_log (ceocl_status)
    WHERE ceocl_status IN ('STARTED', 'UNKNOWN');

COMMIT;
