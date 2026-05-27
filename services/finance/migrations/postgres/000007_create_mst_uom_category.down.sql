-- Reverse migration: Restore UOM category enum approach

-- Step 1: Drop FK index
DROP INDEX IF EXISTS idx_mst_uom_category_id;

-- Step 2: Drop FK constraint
ALTER TABLE mst_uom DROP CONSTRAINT IF EXISTS fk_mst_uom_category;

-- Step 3: Add back the uom_category column
ALTER TABLE mst_uom ADD COLUMN IF NOT EXISTS uom_category VARCHAR(50);

-- Step 4: Migrate data back from FK to string
UPDATE mst_uom u
SET uom_category = c.category_code
FROM mst_uom_category c
WHERE u.uom_category_id = c.uom_category_id;

-- Step 5: Make uom_category NOT NULL and add CHECK constraint
ALTER TABLE mst_uom ALTER COLUMN uom_category SET NOT NULL;
ALTER TABLE mst_uom ADD CONSTRAINT chk_mst_uom_category
    CHECK (uom_category IN ('WEIGHT', 'LENGTH', 'VOLUME', 'QUANTITY'));

-- Step 6: Drop uom_category_id column
ALTER TABLE mst_uom DROP COLUMN IF EXISTS uom_category_id;

-- Step 7: Recreate old index
CREATE INDEX IF NOT EXISTS idx_mst_uom_category ON mst_uom(uom_category) WHERE deleted_at IS NULL;

-- Step 8: Drop UOM Category table indexes
DROP INDEX IF EXISTS idx_mst_uom_category_search;
DROP INDEX IF EXISTS idx_mst_uom_category_created_at;
DROP INDEX IF EXISTS idx_mst_uom_category_active;
DROP INDEX IF EXISTS idx_mst_uom_category_code;

-- Step 9: Drop UOM Category table
DROP TABLE IF EXISTS mst_uom_category;
