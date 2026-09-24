-- 000521 (oil-cost-rm-group M2) — product type -> oil group mapping.
--
-- 1. cost_product_type.cpt_oil_class: the oil class of a product type
--    ('PTY' | 'POY' | 'SUPERBA'). NULL = the type carries no oil and its
--    OIL_RATE / OIL_COST / OIL_GAIN are not resolved by the engine.
-- 2. cost_product_type_oil_group: the oil RM groups ALLOWED for each type, with
--    exactly ONE default per type (partial unique index). Editable from the
--    Product Types page (decision D10).
--
-- Seed (decision D1), joined on CODES only — zero literal ids:
--   PTY             -> class PTY,     groups {202006101 (default)}
--   POY             -> class POY,     groups {202006077 (default)}
--   TCS / TPS / TTS -> class SUPERBA, groups {202006077 (default)}
-- A type or group missing from the database simply produces no row (guarded
-- join), so the migration is a safe no-op on a partially seeded DB.
--
-- Idempotent: ADD COLUMN IF NOT EXISTS, DO-block guarded ADD CONSTRAINT,
-- CREATE TABLE/INDEX IF NOT EXISTS, class UPDATE only on NULL, mapping INSERT
-- ON CONFLICT DO NOTHING.

BEGIN;

-- ============================================================
-- PART 1: cost_product_type.cpt_oil_class
-- ============================================================
ALTER TABLE cost_product_type
    ADD COLUMN IF NOT EXISTS cpt_oil_class VARCHAR(10);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_cpt_oil_class'
    ) THEN
        ALTER TABLE cost_product_type
            ADD CONSTRAINT chk_cpt_oil_class
            CHECK (cpt_oil_class IS NULL OR cpt_oil_class IN ('PTY', 'POY', 'SUPERBA'));
    END IF;
END $$;

COMMENT ON COLUMN cost_product_type.cpt_oil_class IS
    'Oil class of the product type: PTY / POY / SUPERBA. NULL = no oil (OIL_RATE not resolved by the calc engine).';

UPDATE cost_product_type t
SET cpt_oil_class  = v.oil_class,
    cpt_updated_at = NOW()
FROM (VALUES
    ('PTY', 'PTY'),
    ('POY', 'POY'),
    ('TCS', 'SUPERBA'),
    ('TPS', 'SUPERBA'),
    ('TTS', 'SUPERBA')
) AS v(type_code, oil_class)
WHERE t.cpt_type_code = v.type_code
  AND t.cpt_oil_class IS NULL;

-- ============================================================
-- PART 2: cost_product_type_oil_group
-- ============================================================
CREATE TABLE IF NOT EXISTS cost_product_type_oil_group (
    cptog_id            SERIAL       PRIMARY KEY,
    cptog_type_id       INT          NOT NULL
        REFERENCES cost_product_type (cpt_type_id) ON DELETE CASCADE,
    cptog_group_head_id UUID         NOT NULL
        REFERENCES cst_rm_group_head (group_head_id) ON DELETE RESTRICT,
    cptog_is_default    BOOLEAN      NOT NULL DEFAULT FALSE,
    cptog_created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cptog_created_by    VARCHAR(100) NOT NULL,
    cptog_updated_at    TIMESTAMPTZ,
    cptog_updated_by    VARCHAR(100),

    CONSTRAINT uk_cptog_type_group UNIQUE (cptog_type_id, cptog_group_head_id)
);

-- Exactly one default per product type.
CREATE UNIQUE INDEX IF NOT EXISTS uk_cptog_type_default
    ON cost_product_type_oil_group (cptog_type_id)
    WHERE cptog_is_default;

-- Reverse lookup for the "un-flag an in-use oil group" guard (ErrOilGroupInUse).
CREATE INDEX IF NOT EXISTS idx_cptog_group_head
    ON cost_product_type_oil_group (cptog_group_head_id);

COMMENT ON TABLE cost_product_type_oil_group IS
    'Oil RM groups allowed per product type (OIL_NAME options/validation); exactly one default per type.';
COMMENT ON COLUMN cost_product_type_oil_group.cptog_is_default IS
    'TRUE = default OIL_NAME group for the type (written on CAPP add / blank import / calc-time fallback).';

INSERT INTO cost_product_type_oil_group (
    cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by
)
SELECT t.cpt_type_id, g.group_head_id, v.is_default, 'seed_000521'
FROM (VALUES
    ('PTY', '202006101', TRUE),
    ('POY', '202006077', TRUE),
    ('TCS', '202006077', TRUE),
    ('TPS', '202006077', TRUE),
    ('TTS', '202006077', TRUE)
) AS v(type_code, group_code, is_default)
JOIN cost_product_type t ON t.cpt_type_code = v.type_code
JOIN cst_rm_group_head g ON g.group_code = v.group_code AND g.deleted_at IS NULL
ON CONFLICT ON CONSTRAINT uk_cptog_type_group DO NOTHING;

DO $$
DECLARE
    v_classes INT;
    v_rows    INT;
BEGIN
    SELECT COUNT(*) INTO v_classes FROM cost_product_type WHERE cpt_oil_class IS NOT NULL;
    SELECT COUNT(*) INTO v_rows    FROM cost_product_type_oil_group;
    RAISE NOTICE '000521: product types with an oil class = %, oil-group mapping rows = %', v_classes, v_rows;
END $$;

COMMIT;
