-- Migration: Create prd_request + prd_request_sequence for product request tickets.
-- Phase 1: header only. Attachments + comments come in Phase 6.

CREATE TABLE IF NOT EXISTS prd_request (
    request_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_no           VARCHAR(20) NOT NULL,
    requester_id        UUID NOT NULL,
    requester_username  VARCHAR(100) NOT NULL,
    requester_dept_id   UUID NOT NULL,
    requester_dept_code VARCHAR(10),
    title               VARCHAR(200) NOT NULL,
    description         TEXT,
    target_specs        JSONB,
    status              VARCHAR(30) NOT NULL DEFAULT 'OPEN',
    resolved_product_id UUID,
    resolution_note     TEXT,
    reject_reason       TEXT,
    assigned_to         UUID,
    due_date            DATE,
    created_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by          VARCHAR(100) NOT NULL,
    updated_at          TIMESTAMP,
    updated_by          VARCHAR(100),
    deleted_at          TIMESTAMP,
    deleted_by          VARCHAR(100),
    CONSTRAINT chk_prd_request_status
        CHECK (status IN ('OPEN','IN_REVIEW','PRODUCT_PROPOSED','RESOLVED','REJECTED'))
);

-- ticket_no unique only among non-deleted (so soft-deleted tickets free up their no.)
CREATE UNIQUE INDEX IF NOT EXISTS uk_prd_request_ticket_no
    ON prd_request(ticket_no) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prd_request_status
    ON prd_request(status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prd_request_requester
    ON prd_request(requester_id, status);

CREATE INDEX IF NOT EXISTS idx_prd_request_assigned
    ON prd_request(assigned_to, status);

CREATE INDEX IF NOT EXISTS idx_prd_request_resolved_product
    ON prd_request(resolved_product_id);

CREATE INDEX IF NOT EXISTS idx_prd_request_specs_gin
    ON prd_request USING gin(target_specs);

CREATE INDEX IF NOT EXISTS idx_prd_request_fts ON prd_request
    USING gin(to_tsvector('simple',
        coalesce(title,'') || ' ' || coalesce(description,'')))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_prd_request_due_date
    ON prd_request(due_date)
    WHERE status NOT IN ('RESOLVED','REJECTED') AND deleted_at IS NULL;

COMMENT ON TABLE prd_request IS 'Product request tickets raised by Marketing/Production for Finance to fulfill.';
COMMENT ON COLUMN prd_request.ticket_no IS 'Format: PR-YYYYMM-NNN. Allocated atomically via prd_request_sequence.';
COMMENT ON COLUMN prd_request.target_specs IS 'JSONB free-form spec hints (shade, count, blend, etc.).';

-- Atomic ticket-no sequence helper (one row per period YYYYMM)
CREATE TABLE IF NOT EXISTS prd_request_sequence (
    period      VARCHAR(6) PRIMARY KEY,           -- YYYYMM
    last_seq    INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE prd_request_sequence IS 'Atomic counter for ticket numbers. Use INSERT...ON CONFLICT to allocate next.';
