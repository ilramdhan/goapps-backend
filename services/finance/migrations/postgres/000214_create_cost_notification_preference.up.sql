-- Canonical PRD Phase A §7.1.13 — cost_notification_preference (CNP_).
-- Per-user per-trigger notification settings (email, in-app, digest mode).
CREATE TABLE IF NOT EXISTS cost_notification_preference (
    cnp_pref_id        BIGSERIAL    PRIMARY KEY,
    cnp_user_id        VARCHAR(64)  NOT NULL,
    cnp_trigger_type   VARCHAR(50)  NOT NULL,
    cnp_channel_email  BOOLEAN      NOT NULL DEFAULT TRUE,
    cnp_channel_in_app BOOLEAN      NOT NULL DEFAULT TRUE,
    cnp_digest_mode    VARCHAR(20)  NOT NULL DEFAULT 'immediate',
    CONSTRAINT uk_cnp_unique UNIQUE (cnp_user_id, cnp_trigger_type),
    CONSTRAINT chk_cnp_digest_mode CHECK (cnp_digest_mode IN ('immediate', 'daily'))
);

CREATE INDEX IF NOT EXISTS idx_cnp_user ON cost_notification_preference (cnp_user_id);

COMMENT ON TABLE cost_notification_preference IS 'PRD Phase A §7.1.13 — Per-user per-trigger notification preferences.';
