-- Canonical PRD Phase B §7.3.3 — cost_erp_shade (CES_).
-- Read-only replica of Oracle ERP shade master (NL/Z114S/Z108S etc).
-- Also used by Phase A cost_product_spec for shade autocomplete.

CREATE TABLE IF NOT EXISTS cost_erp_shade (
    ces_shade_id   SERIAL       PRIMARY KEY,
    ces_shade_code VARCHAR(20)  NOT NULL,
    ces_shade_name VARCHAR(100),
    ces_is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    ces_synced_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_erp_shade_code
    ON cost_erp_shade (ces_shade_code);

CREATE INDEX IF NOT EXISTS idx_cost_erp_shade_name_search
    ON cost_erp_shade USING GIN (to_tsvector('simple', COALESCE(ces_shade_name, '')));

COMMENT ON TABLE cost_erp_shade IS 'PRD Phase B §7.3.3 — Read-only replica of Oracle ERP shade master. Also used by Phase A cost_product_spec.';
