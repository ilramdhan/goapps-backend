ALTER TABLE cost_erp_item
    DROP COLUMN IF EXISTS cei_created_at,
    DROP COLUMN IF EXISTS cei_updated_at,
    DROP COLUMN IF EXISTS cei_created_by,
    DROP COLUMN IF EXISTS cei_updated_by;
