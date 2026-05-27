-- Canonical PRD Phase A §7.1.9 — cost_request_mention (CRM_).
-- Fast-lookup table for @mentions, used by notification trigger.
CREATE TABLE IF NOT EXISTS cost_request_mention (
    crm_mention_id        BIGSERIAL    PRIMARY KEY,
    crm_comment_id        BIGINT       NOT NULL
        REFERENCES cost_request_comment (crc_comment_id) ON DELETE CASCADE,
    crm_mentioned_user_id VARCHAR(64)  NOT NULL,
    crm_is_notified       BOOLEAN      NOT NULL DEFAULT FALSE,
    crm_notified_at       TIMESTAMPTZ,
    CONSTRAINT uk_crm_unique UNIQUE (crm_comment_id, crm_mentioned_user_id)
);

CREATE INDEX IF NOT EXISTS idx_crm_user_pending
    ON cost_request_mention (crm_mentioned_user_id)
    WHERE crm_is_notified = FALSE;

COMMENT ON TABLE cost_request_mention IS 'PRD Phase A §7.1.9 — @mention index for notification dispatch.';
