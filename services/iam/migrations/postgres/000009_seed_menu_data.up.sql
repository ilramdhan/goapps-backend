-- IAM Service Database Migrations
-- 000009: Seed initial menu data
--
-- Seeds the mst_menu table with the initial navigation structure.
-- Menu items map to the permission system for role-based visibility.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.
--
-- Menu hierarchy (3 levels max):
--   Level 1 (ROOT)   → Top-level modules (Dashboard, Finance, Administrator, etc.)
--   Level 2 (PARENT) → Sub-sections (Master, Transaction, etc.)
--   Level 3 (CHILD)  → Leaf pages (UOM, Costing Process, etc.)

-- =============================================================================
-- LEVEL 1 — ROOT MENUS
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    -- Dashboard (no permission required — visible to all authenticated users)
    ('00000000-0000-0000-0001-000000000001', NULL, 'DASHBOARD',      'Dashboard',      '/dashboard',           'LayoutDashboard', 'iam',     1, 10, true, true, 'seed'),

    -- Modules
    ('00000000-0000-0000-0001-000000000002', NULL, 'FINANCE',        'Finance',        NULL,                   'DollarSign',      'finance', 1, 20, true, true, 'seed'),
    ('00000000-0000-0000-0001-000000000003', NULL, 'IT',             'IT',             NULL,                   'MonitorDot',      'it',      1, 30, true, true, 'seed'),
    ('00000000-0000-0000-0001-000000000004', NULL, 'HR',             'HR',             NULL,                   'Users',           'hr',      1, 40, true, true, 'seed'),
    ('00000000-0000-0000-0001-000000000005', NULL, 'CI',             'CI',             NULL,                   'TrendingUp',      'ci',      1, 50, true, true, 'seed'),
    ('00000000-0000-0000-0001-000000000006', NULL, 'EXSIM',          'Export Import',  NULL,                   'Ship',            'exsim',   1, 60, true, true, 'seed'),

    -- Administrator (IAM management)
    ('00000000-0000-0000-0001-000000000007', NULL, 'ADMINISTRATOR',  'Administrator',  NULL,                   'Settings',        'iam',     1, 70, true, true, 'seed')

ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- LEVEL 2 — PARENT MENUS (children of root menus)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    -- Finance children
    ('00000000-0000-0000-0002-000000000001', '00000000-0000-0000-0001-000000000002', 'FINANCE_DASHBOARD',    'Dashboard',    '/finance/dashboard', 'LayoutDashboard', 'finance', 2, 10, true, true, 'seed'),
    ('00000000-0000-0000-0002-000000000002', '00000000-0000-0000-0001-000000000002', 'FINANCE_MASTER',       'Master',       NULL,                 'Database',        'finance', 2, 20, true, true, 'seed'),
    ('00000000-0000-0000-0002-000000000003', '00000000-0000-0000-0001-000000000002', 'FINANCE_TRANSACTION',  'Transaction',  NULL,                 'Receipt',         'finance', 2, 30, true, true, 'seed'),

    -- IT children
    ('00000000-0000-0000-0002-000000000004', '00000000-0000-0000-0001-000000000003', 'IT_DASHBOARD',         'Dashboard',    '/it/dashboard',      'LayoutDashboard', 'it',      2, 10, true, true, 'seed'),

    -- HR children
    ('00000000-0000-0000-0002-000000000005', '00000000-0000-0000-0001-000000000004', 'HR_DASHBOARD',         'Dashboard',    '/hr/dashboard',      'LayoutDashboard', 'hr',      2, 10, true, true, 'seed'),

    -- CI children
    ('00000000-0000-0000-0002-000000000006', '00000000-0000-0000-0001-000000000005', 'CI_DASHBOARD',         'Dashboard',    '/ci/dashboard',      'LayoutDashboard', 'ci',      2, 10, true, true, 'seed'),

    -- Export Import children
    ('00000000-0000-0000-0002-000000000007', '00000000-0000-0000-0001-000000000006', 'EXSIM_DASHBOARD',      'Dashboard',    '/exsim/dashboard',   'LayoutDashboard', 'exsim',   2, 10, true, true, 'seed'),

    -- Administrator children
    ('00000000-0000-0000-0002-000000000008', '00000000-0000-0000-0001-000000000007', 'ADMIN_USERS',          'User Management',       '/administrator/users',  'Users',        'iam', 2, 10, true, true, 'seed'),
    ('00000000-0000-0000-0002-000000000009', '00000000-0000-0000-0001-000000000007', 'ADMIN_ROLES',          'Roles & Permissions',   '/administrator/roles',  'Shield',       'iam', 2, 20, true, true, 'seed'),
    ('00000000-0000-0000-0002-000000000010', '00000000-0000-0000-0001-000000000007', 'ADMIN_MENUS',          'Menu Management',       '/administrator/menus',  'Menu',         'iam', 2, 30, true, true, 'seed'),
    ('00000000-0000-0000-0002-000000000011', '00000000-0000-0000-0001-000000000007', 'ADMIN_PERMISSIONS',    'Permission Management', '/administrator/permissions', 'Shield',  'iam', 2, 25, true, true, 'seed')

ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- LEVEL 3 — CHILD MENUS (leaf pages)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    -- Finance Master children
    ('00000000-0000-0000-0003-000000000001', '00000000-0000-0000-0002-000000000002', 'FINANCE_UOM',          'Unit of Measure',  '/finance/master/uom',                    'Ruler',         'finance', 3, 10, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000002', '00000000-0000-0000-0002-000000000002', 'FINANCE_PARAMETERS',   'Parameters',       '/finance/master/parameters',             'SlidersHorizontal', 'finance', 3, 20, true, true, 'seed'),

    -- Finance Transaction children
    ('00000000-0000-0000-0003-000000000003', '00000000-0000-0000-0002-000000000003', 'FINANCE_COSTING',      'Costing Process',  '/finance/transaction/costing-process',   'Calculator',    'finance', 3, 10, true, true, 'seed')

ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link menus to required permissions
-- =============================================================================
-- A user needs ANY ONE of the linked permissions to see the menu.
-- Dashboard menu has NO permissions → visible to all authenticated users.

-- Finance root → requires finance.module.root.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000002', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.module.root.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance Dashboard → requires finance.module.dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000001', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.module.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance Master → requires finance.module.master.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000002', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.module.master.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance Transaction → requires finance.module.transaction.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000003', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.module.transaction.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance UOM → requires finance.master.uom.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000001', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.uom.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance Parameters → requires finance.master.parameters.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000002', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.parameters.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Finance Costing Process → requires finance.transaction.costing.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000003', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.transaction.costing.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- IT root → requires it.module.root.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000003', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'it.module.root.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- IT Dashboard → requires it.module.dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000004', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'it.module.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- HR root → requires hr.module.root.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000004', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'hr.module.root.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- HR Dashboard → requires hr.module.dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000005', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'hr.module.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- CI root → requires ci.module.root.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000005', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'ci.module.root.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- CI Dashboard → requires ci.module.dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000006', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'ci.module.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- EXSIM root → requires exsim.module.root.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000006', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'exsim.module.root.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- EXSIM Dashboard → requires exsim.module.dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000007', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'exsim.module.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Administrator root → requires iam.user.account.view OR iam.rbac.role.view OR iam.menu.menu.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0001-000000000007', permission_id, 'seed'
FROM mst_permission
WHERE permission_code IN ('iam.user.account.view', 'iam.rbac.role.view', 'iam.menu.menu.view')
    AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Admin Users → requires iam.user.account.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000008', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.user.account.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Admin Roles → requires iam.rbac.role.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000009', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.rbac.role.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Admin Permissions → requires iam.rbac.permission.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000011', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.rbac.permission.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Admin Menus → requires iam.menu.menu.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000010', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.menu.menu.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;
