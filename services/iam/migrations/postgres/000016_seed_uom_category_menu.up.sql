-- IAM Service Database Migrations
-- 000016: Seed UOM Category menu and permissions
--
-- Adds UOM Category as a child of Finance > Master in the sidebar navigation.
-- Also seeds the finance.master.uomcategory.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.master.uomcategory.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.uomcategory.view',   'View UOM Categories',   'View UOM categories list and details',   'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.uomcategory.create', 'Create UOM Category',   'Create new UOM categories',              'finance', 'master', 'create', true, 'seed'),
    ('finance.master.uomcategory.update', 'Update UOM Category',   'Update existing UOM categories',         'finance', 'master', 'update', true, 'seed'),
    ('finance.master.uomcategory.delete', 'Delete UOM Category',   'Delete UOM categories',                  'finance', 'master', 'delete', true, 'seed'),
    ('finance.master.uomcategory.export', 'Export UOM Categories',  'Export UOM categories to Excel',         'finance', 'master', 'export', true, 'seed'),
    ('finance.master.uomcategory.import', 'Import UOM Categories',  'Import UOM categories from Excel',       'finance', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > UOM Category
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000007', '00000000-0000-0000-0002-000000000002', 'FINANCE_UOM_CATEGORY', 'UOM Category', '/finance/master/uom-category', 'FolderTree', 'finance', 3, 12, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link UOM Category menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000007', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.uomcategory.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN UOM CATEGORY PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================
-- Ensures the super admin role (if it exists) gets all UOM Category permissions.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.uomcategory.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
