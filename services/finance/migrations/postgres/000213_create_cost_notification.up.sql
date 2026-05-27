-- Canonical PRD Phase A §7.1.12 — cost_notification (CN_).
-- In-app notification record with optional email-sent timestamp. Mentions, status
-- changes, assignments, feasibility decisions etc. all funnel here.
CREATE TABLE IF NOT EXISTS cost_notification (
    cn_notification_id  BIGSERIAL    PRIMARY KEY,
    cn_recipient_user_id VARCHAR(64) NOT NULL,
    cn_trigger_type     VARCHAR(50)  NOT NULL,
    cn_request_id       BIGINT
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    cn_payload          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    cn_is_read          BOOLEAN      NOT NULL DEFAULT FALSE,
    cn_email_sent_at    TIMESTAMPTZ,
    cn_created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_cn_trigger_type CHECK (
        cn_trigger_type IN (
            'STATUS_CHANGE', 'MENTION', 'ASSIGNED', 'FEASIBILITY',
            'COMMENT_ADDED', 'ROUTING_PROMOTED', 'REQUEST_REJECTED', 'REQUEST_CLOSED'
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_cn_recipient_unread
    ON cost_notification (cn_recipient_user_id, cn_created_at DESC)
    WHERE cn_is_read = FALSE;

CREATE INDEX IF NOT EXISTS idx_cn_recipient_all
    ON cost_notification (cn_recipient_user_id, cn_created_at DESC);

COMMENT ON TABLE cost_notification IS 'PRD Phase A §7.1.12 — In-app notification record.';
