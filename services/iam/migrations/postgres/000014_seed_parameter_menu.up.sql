-- IAM Service Database Migrations
-- 000014: Seed Parameter menu and permissions
--
-- Adds Parameter as a child of Finance > Master in the sidebar navigation.
-- Also seeds the finance.master.parameter.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.master.parameter.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.parameter.view',   'View Parameters',   'View parameter list and details',    'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.parameter.create', 'Create Parameter',  'Create new parameters',              'finance', 'master', 'create', true, 'seed'),
    ('finance.master.parameter.update', 'Update Parameter',  'Update existing parameters',         'finance', 'master', 'update', true, 'seed'),
    ('finance.master.parameter.delete', 'Delete Parameter',  'Delete parameters',                  'finance', 'master', 'delete', true, 'seed'),
    ('finance.master.parameter.export', 'Export Parameters',  'Export parameters to Excel',         'finance', 'master', 'export', true, 'seed'),
    ('finance.master.parameter.import', 'Import Parameters',  'Import parameters from Excel',       'finance', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > Parameter
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000005', '00000000-0000-0000-0002-000000000002', 'FINANCE_PARAMETER', 'Parameter', '/finance/master/parameter', 'SlidersHorizontal', 'finance', 3, 20, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link Parameter menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000005', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.parameter.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN PARAMETER PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.parameter.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
