-- IAM Service Database Migrations
-- 000015: Seed Formula menu and permissions
--
-- Adds Formula as a child of Finance > Master in the sidebar navigation.
-- Also seeds the finance.master.formula.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.master.formula.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.formula.view',   'View Formulas',   'View formula list and details',    'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.formula.create', 'Create Formula',  'Create new formulas',              'finance', 'master', 'create', true, 'seed'),
    ('finance.master.formula.update', 'Update Formula',  'Update existing formulas',         'finance', 'master', 'update', true, 'seed'),
    ('finance.master.formula.delete', 'Delete Formula',  'Delete formulas',                  'finance', 'master', 'delete', true, 'seed'),
    ('finance.master.formula.export', 'Export Formulas',  'Export formulas to Excel',         'finance', 'master', 'export', true, 'seed'),
    ('finance.master.formula.import', 'Import Formulas',  'Import formulas from Excel',       'finance', 'master', 'import', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > Formula
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000006', '00000000-0000-0000-0002-000000000002', 'FINANCE_FORMULA', 'Formula', '/finance/master/formula', 'FunctionSquare', 'finance', 3, 30, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link Formula menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000006', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.formula.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN FORMULA PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.formula.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
