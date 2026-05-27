-- Migration: Create mst_uom_category table and migrate UOM category from enum to FK
-- Description: Master table for UOM Categories, replaces hardcoded UOMCategory enum

-- Step 1: Create mst_uom_category table
CREATE TABLE IF NOT EXISTS mst_uom_category (
    -- Primary Key
    uom_category_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Core Fields
    category_code VARCHAR(20) NOT NULL,
    category_name VARCHAR(100) NOT NULL,
    description TEXT,

    -- Status
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Audit Fields
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),

    -- Soft Delete Fields
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(100),

    -- Constraints
    CONSTRAINT uq_mst_uom_category_code UNIQUE (category_code)
);

-- Comments
COMMENT ON TABLE mst_uom_category IS 'Master table for UOM Categories';
COMMENT ON COLUMN mst_uom_category.uom_category_id IS 'Unique identifier (UUID)';
COMMENT ON COLUMN mst_uom_category.category_code IS 'Unique code (e.g., WEIGHT, LENGTH, VOLUME)';
COMMENT ON COLUMN mst_uom_category.category_name IS 'Display name (e.g., Weight, Length, Volume)';
COMMENT ON COLUMN mst_uom_category.is_active IS 'Whether the category is active';
COMMENT ON COLUMN mst_uom_category.deleted_at IS 'Soft delete timestamp (NULL = not deleted)';

-- Indexes for common queries (excluding soft-deleted records)
CREATE INDEX IF NOT EXISTS idx_mst_uom_category_code ON mst_uom_category(category_code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_mst_uom_category_active ON mst_uom_category(is_active) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_mst_uom_category_created_at ON mst_uom_category(created_at) WHERE deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_uom_category_search ON mst_uom_category
    USING gin(to_tsvector('english', coalesce(category_code, '') || ' ' || coalesce(category_name, '') || ' ' || coalesce(description, '')))
    WHERE deleted_at IS NULL;

-- Step 2: Seed initial UOM categories
INSERT INTO mst_uom_category (category_code, category_name, description, created_by) VALUES
    ('WEIGHT', 'Weight', 'Weight-based units (e.g., KG, GR, TON)', 'system'),
    ('LENGTH', 'Length', 'Length-based units (e.g., MTR, CM, YARD)', 'system'),
    ('VOLUME', 'Volume', 'Volume-based units (e.g., LTR, ML)', 'system'),
    ('QUANTITY', 'Quantity', 'Quantity-based units (e.g., PCS, BOX, SET)', 'system'),
    ('TIME', 'Time', 'Time-based units (e.g., HR, MIN, SEC)', 'system'),
    ('AREA', 'Area', 'Area-based units (e.g., M2, HA, ACRE)', 'system'),
    ('ENERGY', 'Energy', 'Energy-based units (e.g., KWH, MJ, CAL)', 'system'),
    ('TEMPERATURE', 'Temperature', 'Temperature units (e.g., CEL, FAH, KEL)', 'system'),
    ('PRESSURE', 'Pressure', 'Pressure-based units (e.g., BAR, PSI, ATM)', 'system')
ON CONFLICT (category_code) DO NOTHING;

-- Step 3: Add uom_category_id column to mst_uom
ALTER TABLE mst_uom ADD COLUMN IF NOT EXISTS uom_category_id UUID;

-- Step 4: Migrate existing data from uom_category string to uom_category_id FK
UPDATE mst_uom u
SET uom_category_id = c.uom_category_id
FROM mst_uom_category c
WHERE UPPER(u.uom_category) = c.category_code
  AND u.uom_category_id IS NULL;

-- Step 5: Drop the old CHECK constraint and column
ALTER TABLE mst_uom DROP CONSTRAINT IF EXISTS chk_mst_uom_category;
ALTER TABLE mst_uom DROP COLUMN IF EXISTS uom_category;

-- Step 6: Make uom_category_id NOT NULL after migration
ALTER TABLE mst_uom ALTER COLUMN uom_category_id SET NOT NULL;

-- Step 7: Add foreign key constraint
ALTER TABLE mst_uom ADD CONSTRAINT fk_mst_uom_category
    FOREIGN KEY (uom_category_id)
    REFERENCES mst_uom_category(uom_category_id);

-- Step 8: Create index on FK column
CREATE INDEX IF NOT EXISTS idx_mst_uom_category_id ON mst_uom(uom_category_id) WHERE deleted_at IS NULL;

-- Drop old category index (no longer needed)
DROP INDEX IF EXISTS idx_mst_uom_category;
