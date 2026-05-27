-- Canonical PRD Phase B §7.3.1 — cost_erp_item (CEI_).
-- Read-only replica of Oracle ERP master_item. Sync mechanism is infrastructure-level
-- (CDC or scheduled job) — not part of this schema.

CREATE TABLE IF NOT EXISTS cost_erp_item (
    cei_item_id   BIGSERIAL    PRIMARY KEY,
    cei_item_code VARCHAR(20)  NOT NULL,
    cei_item_name VARCHAR(255),
    cei_item_type VARCHAR(10),
    cei_is_active BOOLEAN      NOT NULL DEFAULT TRUE,
    cei_synced_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_erp_item_code
    ON cost_erp_item (cei_item_code);

CREATE INDEX IF NOT EXISTS idx_cost_erp_item_type
    ON cost_erp_item (cei_item_type)
    WHERE cei_is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_cost_erp_item_name_search
    ON cost_erp_item USING GIN (to_tsvector('simple', COALESCE(cei_item_name, '')));

COMMENT ON TABLE cost_erp_item IS 'PRD Phase B §7.3.1 — Read-only replica of Oracle ERP master_item. Source of truth for Store-Rate components.';
