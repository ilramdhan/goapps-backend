-- Create mst_notification table — generic notification system used by IAM and
-- emitted-into by other services (e.g. finance worker emitting EXPORT_READY).

CREATE TABLE IF NOT EXISTS mst_notification (
    notification_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_user_id UUID NOT NULL REFERENCES mst_user(user_id) ON DELETE CASCADE,

    type              VARCHAR(40) NOT NULL,
    severity          VARCHAR(20) NOT NULL DEFAULT 'INFO',
    title             VARCHAR(200) NOT NULL,
    body              TEXT,

    action_type       VARCHAR(40) NOT NULL DEFAULT 'NONE',
    action_payload    JSONB,

    status            VARCHAR(20) NOT NULL DEFAULT 'UNREAD',
    read_at           TIMESTAMPTZ,
    archived_at       TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ,

    source_type       VARCHAR(50),
    source_id         VARCHAR(100),

    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by        VARCHAR(100) NOT NULL DEFAULT 'system',

    CONSTRAINT chk_notif_type CHECK (type IN (
        'EXPORT_READY','ALERT','APPROVAL','CHAT','REMINDER','SYSTEM','MENTION','ASSIGNMENT','ANNOUNCEMENT'
    )),
    CONSTRAINT chk_notif_severity CHECK (severity IN ('INFO','SUCCESS','WARNING','ERROR')),
    CONSTRAINT chk_notif_action CHECK (action_type IN (
        'NONE','NAVIGATE','DOWNLOAD','EXTERNAL_LINK',
        'APPROVE_REJECT','ACKNOWLEDGE','MULTI_ACTION',
        'REPLY','SNOOZE','CUSTOM'
    )),
    CONSTRAINT chk_notif_status CHECK (status IN ('UNREAD','READ','ARCHIVED'))
);

CREATE INDEX IF NOT EXISTS idx_notif_recipient_status_created ON mst_notification (recipient_user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notif_recipient_created        ON mst_notification (recipient_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notif_expires                  ON mst_notification (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_notif_source                   ON mst_notification (source_type, source_id);

COMMENT ON TABLE mst_notification IS
    'Generic per-user notification store. Any service can call IAM gRPC CreateNotification. action_type drives FE rendering; action_payload JSON shape depends on action_type.';
