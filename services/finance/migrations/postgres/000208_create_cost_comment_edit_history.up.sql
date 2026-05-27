-- Canonical PRD Phase A §7.1.8 — cost_comment_edit_history (CCEH_).
-- Append-only edit snapshots of cost_request_comment (transparency for FR-5).
CREATE TABLE IF NOT EXISTS cost_comment_edit_history (
    cceh_edit_id        BIGSERIAL    PRIMARY KEY,
    cceh_comment_id     BIGINT       NOT NULL
        REFERENCES cost_request_comment (crc_comment_id) ON DELETE CASCADE,
    cceh_body_richtext  JSONB        NOT NULL,
    cceh_body_plaintext TEXT         NOT NULL,
    cceh_edited_by      VARCHAR(64)  NOT NULL,
    cceh_edited_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cceh_comment ON cost_comment_edit_history (cceh_comment_id, cceh_edited_at DESC);

COMMENT ON TABLE cost_comment_edit_history IS 'PRD Phase A §7.1.8 — Append-only snapshot of comment bodies prior to each edit.';
