-- IAM Service Database Migrations
-- 000011: Seed RM Category menu and permissions
--
-- Adds RM Category as a child of Finance > Master in the sidebar navigation.
-- Also seeds the finance.master.rmcategory.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.master.rmcategory.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.rmcategory.view',   'View RM Categories',   'View raw material categories list and details',   'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.rmcategory.create', 'Create RM Category',   'Create new raw material categories',              'finance', 'master', 'create', true, 'seed'),
    ('finance.master.rmcategory.update', 'Update RM Category',   'Update existing raw material categories',         'finance', 'master', 'update', true, 'seed'),
    ('finance.master.rmcategory.delete', 'Delete RM Category',   'Delete raw material categories',                  'finance', 'master', 'delete', true, 'seed'),
    ('finance.master.rmcategory.export', 'Export RM Categories',  'Export raw material categories to Excel',         'finance', 'master', 'export', true, 'seed'),
    ('finance.master.rmcategory.import', 'Import RM Categories',  'Import raw material categories from Excel',       'finance', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > RM Category
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000004', '00000000-0000-0000-0002-000000000002', 'FINANCE_RM_CATEGORY', 'RM Category', '/finance/master/rm-category', 'Layers', 'finance', 3, 15, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link RM Category menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000004', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.rmcategory.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN RM CATEGORY PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================
-- Ensures the super admin role (if it exists) gets all RM Category permissions.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.rmcategory.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
