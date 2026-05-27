-- IAM Service Database Migrations
-- 000024: Seed Employee Group menu and permissions
--
-- Adds "Employee Group" under "Master Data" section.
-- Seeds iam.master.employeegroup.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — iam.master.employeegroup.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.master.employeegroup.view',   'View Employee Groups',   'View employee groups list and details', 'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.employeegroup.create', 'Create Employee Group',  'Create new employee groups',            'iam', 'master', 'create', true, 'seed'),
    ('iam.master.employeegroup.update', 'Update Employee Group',  'Update existing employee groups',       'iam', 'master', 'update', true, 'seed'),
    ('iam.master.employeegroup.delete', 'Delete Employee Group',  'Delete employee groups',                'iam', 'master', 'delete', true, 'seed'),
    ('iam.master.employeegroup.export', 'Export Employee Groups', 'Export employee groups to Excel',       'iam', 'master', 'export', true, 'seed'),
    ('iam.master.employeegroup.import', 'Import Employee Groups', 'Import employee groups from Excel',     'iam', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU — Administrator > Master Data > Employee Group (Level 3 leaf)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000009', '00000000-0000-0000-0002-000000000013', 'ADMIN_EMPLOYEE_GROUP', 'Employee Group', '/administrator/master/employee-groups', 'Users', 'iam', 3, 20, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link Employee Group menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000009', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.master.employeegroup.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN EMPLOYEE GROUP PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'iam.master.employeegroup.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
