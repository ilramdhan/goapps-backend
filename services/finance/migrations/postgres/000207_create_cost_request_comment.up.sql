-- Canonical PRD Phase A §7.1.7 — cost_request_comment (CRC_).
-- Rich-text comment thread on a product request. body_richtext = Tiptap/Lexical JSON
-- tree; body_plaintext is the searchable + notification copy.
-- NOTE: G4 (encrypted chat) is deferred — the canonical PRD specifies plain JSON+text
-- columns; encryption can be added later as application-layer transparent encryption
-- without changing this schema.
CREATE TABLE IF NOT EXISTS cost_request_comment (
    crc_comment_id        BIGSERIAL    PRIMARY KEY,
    crc_request_id        BIGINT       NOT NULL
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    crc_parent_comment_id BIGINT
        REFERENCES cost_request_comment (crc_comment_id) ON DELETE SET NULL,
    crc_author_user_id    VARCHAR(64)  NOT NULL,
    crc_body_richtext     JSONB        NOT NULL,
    crc_body_plaintext    TEXT         NOT NULL,
    crc_is_edited         BOOLEAN      NOT NULL DEFAULT FALSE,
    crc_is_hidden         BOOLEAN      NOT NULL DEFAULT FALSE,
    crc_hidden_reason     TEXT,
    crc_created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    crc_updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_crc_hidden_reason CHECK (
        crc_is_hidden = FALSE OR crc_hidden_reason IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_crc_request ON cost_request_comment (crc_request_id);
CREATE INDEX IF NOT EXISTS idx_crc_author  ON cost_request_comment (crc_author_user_id);
CREATE INDEX IF NOT EXISTS idx_crc_search
    ON cost_request_comment USING GIN (to_tsvector('simple', crc_body_plaintext));

COMMENT ON TABLE cost_request_comment IS 'PRD Phase A §7.1.7 — Rich-text comment thread on cost_product_request.';
