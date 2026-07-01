ALTER TABLE mst_permission
  ADD COLUMN IF NOT EXISTS menu_id UUID NULL REFERENCES mst_menu(menu_id);

CREATE INDEX IF NOT EXISTS idx_permission_menu
  ON mst_permission(menu_id)
  WHERE is_active = true;
