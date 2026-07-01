DROP INDEX IF EXISTS idx_permission_menu;
ALTER TABLE mst_permission DROP COLUMN IF EXISTS menu_id;
