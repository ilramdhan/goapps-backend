-- Encrypted chat per request (app-layer AES-256-GCM).
-- PRD v1.3 §0.1 D8.

CREATE TABLE IF NOT EXISTS chat_thread (
    thread_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_kind  VARCHAR(40) NOT NULL,
    entity_id    UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by   VARCHAR(100) NOT NULL,
    archived_at  TIMESTAMPTZ,
    CONSTRAINT chk_chat_thread_entity_kind CHECK (entity_kind IN ('PRD_REQUEST','CST_PRODUCT'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_chat_thread_entity ON chat_thread (entity_kind, entity_id);
CREATE INDEX IF NOT EXISTS idx_chat_thread_archived ON chat_thread (archived_at) WHERE archived_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS chat_message (
    message_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       UUID NOT NULL REFERENCES chat_thread(thread_id) ON DELETE CASCADE,
    sender_user_id  UUID NOT NULL,
    ciphertext      BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    key_version     INT NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    edited_at       TIMESTAMPTZ,
    deleted_at      TIMESTAMPTZ,
    deleted_by      VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_chat_message_thread_sent ON chat_message (thread_id, sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_message_sender ON chat_message (sender_user_id);

CREATE TABLE IF NOT EXISTS chat_thread_participant (
    thread_id     UUID NOT NULL REFERENCES chat_thread(thread_id) ON DELETE CASCADE,
    user_id       UUID NOT NULL,
    joined_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_read_at  TIMESTAMPTZ,
    PRIMARY KEY (thread_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_participant_user ON chat_thread_participant (user_id);
