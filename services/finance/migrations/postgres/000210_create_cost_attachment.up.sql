-- Canonical PRD Phase A §7.1.10 — cost_attachment (CA_).
-- Generic attachment that can hang off either a request OR a comment, never both.
CREATE TABLE IF NOT EXISTS cost_attachment (
    ca_attachment_id BIGSERIAL    PRIMARY KEY,
    ca_request_id    BIGINT
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    ca_comment_id    BIGINT
        REFERENCES cost_request_comment (crc_comment_id) ON DELETE CASCADE,
    ca_filename      VARCHAR(255) NOT NULL,
    ca_mime_type     VARCHAR(100) NOT NULL,
    ca_size_bytes    BIGINT       NOT NULL,
    ca_storage_key   VARCHAR(500) NOT NULL,
    ca_uploaded_by   VARCHAR(64)  NOT NULL,
    ca_uploaded_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_ca_owner_xor CHECK (
        (ca_request_id IS NOT NULL)::INT + (ca_comment_id IS NOT NULL)::INT = 1
    ),
    CONSTRAINT chk_ca_size_positive CHECK (ca_size_bytes > 0)
);

CREATE INDEX IF NOT EXISTS idx_ca_request ON cost_attachment (ca_request_id) WHERE ca_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_ca_comment ON cost_attachment (ca_comment_id) WHERE ca_comment_id IS NOT NULL;

COMMENT ON TABLE cost_attachment IS 'PRD Phase A §7.1.10 — File attachments on requests or comments. Exactly one owner FK is non-null.';
