-- IAM Service Database Migrations
-- 000019: Seed Employee Level menu and permissions
--
-- Adds "Master Data" section under Administrator, and "Employee Level" as a child.
-- Also seeds the iam.master.employeelevel.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — iam.master.employeelevel.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.master.employeelevel.view',   'View Employee Levels',   'View employee levels list and details', 'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.employeelevel.create', 'Create Employee Level',  'Create new employee levels',            'iam', 'master', 'create', true, 'seed'),
    ('iam.master.employeelevel.update', 'Update Employee Level',  'Update existing employee levels',       'iam', 'master', 'update', true, 'seed'),
    ('iam.master.employeelevel.delete', 'Delete Employee Level',  'Delete employee levels',                'iam', 'master', 'delete', true, 'seed'),
    ('iam.master.employeelevel.export', 'Export Employee Levels', 'Export employee levels to Excel',       'iam', 'master', 'export', true, 'seed'),
    ('iam.master.employeelevel.import', 'Import Employee Levels', 'Import employee levels from Excel',     'iam', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU — Administrator > Master Data (Level 2 group)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000013', '00000000-0000-0000-0001-000000000007', 'ADMIN_MASTER_DATA', 'Master Data', NULL, 'Database', 'iam', 2, 40, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU — Administrator > Master Data > Employee Level (Level 3 leaf)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000008', '00000000-0000-0000-0002-000000000013', 'ADMIN_EMPLOYEE_LEVEL', 'Employee Level', '/administrator/master/employee-levels', 'Layers', 'iam', 3, 10, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link Employee Level menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000008', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.master.employeelevel.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN EMPLOYEE LEVEL PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'iam.master.employeelevel.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
