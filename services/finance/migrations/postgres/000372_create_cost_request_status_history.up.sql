-- Migration 000372: Approval trace history for cost product requests.
-- Records every status transition with actor identity (denormalized display name
-- at time of action) for auditing and UI timeline display.
CREATE TABLE IF NOT EXISTS cost_request_status_history (
    crsh_id            BIGSERIAL    PRIMARY KEY,
    crsh_request_id    BIGINT       NOT NULL
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    crsh_from_status   VARCHAR(50),            -- NULL for the initial DRAFT entry
    crsh_to_status     VARCHAR(50)  NOT NULL,
    crsh_actor_user_id VARCHAR(36)  NOT NULL,  -- UUID string of the actor
    crsh_actor_name    VARCHAR(200) NOT NULL,  -- display name at time of action (snapshot)
    crsh_note          TEXT,                   -- optional; reason for REJECTED / CLOSED
    crsh_created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- Composite index supports the primary query pattern:
-- WHERE crsh_request_id = $1 ORDER BY crsh_created_at ASC
CREATE INDEX IF NOT EXISTS idx_crsh_request_id_created_at
    ON cost_request_status_history(crsh_request_id, crsh_created_at ASC);
