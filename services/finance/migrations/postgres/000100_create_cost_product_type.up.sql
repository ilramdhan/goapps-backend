-- Canonical PRD Phase B §7.2.1 — cost_product_type (CPT_).
-- Master of product type (POY/PTY/TTY/etc) that drives auto-generated product code prefix.
-- Also migrates 4 seeded rows from legacy mst_product_type then drops mst_product_type.

CREATE TABLE IF NOT EXISTS cost_product_type (
    cpt_type_id    SERIAL       PRIMARY KEY,
    cpt_type_code  VARCHAR(5)   NOT NULL,
    cpt_type_name  VARCHAR(100) NOT NULL,
    cpt_is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    cpt_created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpt_updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_product_type_code
    ON cost_product_type (cpt_type_code);

-- Migrate existing rows from legacy mst_product_type (only first run; safe on re-run
-- because target uses INSERT ... ON CONFLICT and mst_product_type may already be dropped).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'mst_product_type' AND table_schema = 'public'
    ) THEN
        INSERT INTO cost_product_type (
            cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at
        )
        SELECT type_code, type_name, is_active, created_at, COALESCE(updated_at, created_at)
        FROM mst_product_type
        ON CONFLICT (cpt_type_code) DO NOTHING;

        DROP TABLE mst_product_type CASCADE;
    END IF;
END$$;

COMMENT ON TABLE  cost_product_type IS 'PRD Phase B §7.2.1 — Master of product types (POY/PTY/TTY/etc). Drives product code prefix.';
COMMENT ON COLUMN cost_product_type.cpt_type_code IS 'Short uppercase code, max 5 chars. Used as part of generated product code.';
