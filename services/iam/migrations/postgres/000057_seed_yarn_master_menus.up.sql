-- IAM Service Database Migrations
-- 000057: Seed Yarn Master menus and permissions
--
-- Adds a "Yarn Master" section under Finance (level-2) with 6 leaf menus (level-3).
-- Also seeds the finance.yarnmaster.{entity}.{action} permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.
--
-- UUID allocation:
--   Level-2: 00000000-0000-0000-0002-000000000016  (FINANCE_YARN_MASTER)
--   Level-3: 00000000-0000-0000-0003-000000000021  (FINANCE_MACHINE)
--             00000000-0000-0000-0003-000000000022  (FINANCE_INTERMINGLING)
--             00000000-0000-0000-0003-000000000023  (FINANCE_PRODUCT_GRADE)
--             00000000-0000-0000-0003-000000000024  (FINANCE_MB_HEAD)
--             00000000-0000-0000-0003-000000000025  (FINANCE_MB_SPIN)
--             00000000-0000-0000-0003-000000000026  (FINANCE_BOX_BOBBIN_COST)
--
-- Permission code segments must match [a-z][a-z0-9]* (no underscores/hyphens).

-- =============================================================================
-- PERMISSIONS — finance.yarnmaster.{entity}.{action}
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    -- Machine
    ('finance.yarnmaster.machine.view',   'View Machines',         'View machine list and details',         'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.machine.create', 'Create Machine',        'Create new machines',                   'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.machine.update', 'Update Machine',        'Update existing machines',              'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.machine.delete', 'Delete Machine',        'Delete machines',                       'finance', 'yarnmaster', 'delete', true, 'seed'),
    -- Intermingling
    ('finance.yarnmaster.intermingling.view',   'View Interminglings',   'View intermingling list and details',   'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.intermingling.create', 'Create Intermingling',  'Create new interminglings',             'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.intermingling.update', 'Update Intermingling',  'Update existing interminglings',        'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.intermingling.delete', 'Delete Intermingling',  'Delete interminglings',                 'finance', 'yarnmaster', 'delete', true, 'seed'),
    -- Product Grade
    ('finance.yarnmaster.productgrade.view',   'View Product Grades',   'View product grade list and details',   'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.productgrade.create', 'Create Product Grade',  'Create new product grades',             'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.productgrade.update', 'Update Product Grade',  'Update existing product grades',        'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.productgrade.delete', 'Delete Product Grade',  'Delete product grades',                 'finance', 'yarnmaster', 'delete', true, 'seed'),
    -- MB Head
    ('finance.yarnmaster.mbhead.view',   'View MB Heads',   'View MB head list and details',   'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.mbhead.create', 'Create MB Head',  'Create new MB heads',             'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.mbhead.update', 'Update MB Head',  'Update existing MB heads',        'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.mbhead.delete', 'Delete MB Head',  'Delete MB heads',                 'finance', 'yarnmaster', 'delete', true, 'seed'),
    -- MB Spin
    ('finance.yarnmaster.mbspin.view',   'View MB Spins',   'View MB spin list and details',   'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.mbspin.create', 'Create MB Spin',  'Create new MB spins',             'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.mbspin.update', 'Update MB Spin',  'Update existing MB spins',        'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.mbspin.delete', 'Delete MB Spin',  'Delete MB spins',                 'finance', 'yarnmaster', 'delete', true, 'seed'),
    -- Box/Bobbin Cost
    ('finance.yarnmaster.boxbobbincost.view',   'View Box/Bobbin Costs',   'View box/bobbin cost list and details',   'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.boxbobbincost.create', 'Create Box/Bobbin Cost',  'Create new box/bobbin costs',             'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.boxbobbincost.update', 'Update Box/Bobbin Cost',  'Update existing box/bobbin costs',        'finance', 'yarnmaster', 'update', true, 'seed'),
    ('finance.yarnmaster.boxbobbincost.delete', 'Delete Box/Bobbin Cost',  'Delete box/bobbin costs',                 'finance', 'yarnmaster', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU — Finance > Yarn Master (Level 2 group)
-- Parent: FINANCE root (00000000-0000-0000-0001-000000000002)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000016', '00000000-0000-0000-0001-000000000002', 'FINANCE_YARN_MASTER', 'Yarn Master', NULL, 'Layers', 'finance', 2, 60, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENUS — Finance > Yarn Master > leaf pages (Level 3)
-- Parent: FINANCE_YARN_MASTER (00000000-0000-0000-0002-000000000016)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000021', '00000000-0000-0000-0002-000000000016', 'FINANCE_MACHINE',        'Machine',        '/finance/yarn-master/machines',        'Cpu',          'finance', 3, 10, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000022', '00000000-0000-0000-0002-000000000016', 'FINANCE_INTERMINGLING',  'Intermingling',  '/finance/yarn-master/interminglings',  'Wind',         'finance', 3, 20, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000023', '00000000-0000-0000-0002-000000000016', 'FINANCE_PRODUCT_GRADE',  'Product Grade',  '/finance/yarn-master/product-grades',  'Star',         'finance', 3, 30, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000024', '00000000-0000-0000-0002-000000000016', 'FINANCE_MB_HEAD',        'MB Head',        '/finance/yarn-master/mb-heads',        'FlaskConical', 'finance', 3, 40, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000025', '00000000-0000-0000-0002-000000000016', 'FINANCE_MB_SPIN',        'MB Spin',        '/finance/yarn-master/mb-spins',        'RotateCcw',    'finance', 3, 50, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000026', '00000000-0000-0000-0002-000000000016', 'FINANCE_BOX_BOBBIN_COST','Box/Bobbin Cost','/finance/yarn-master/box-bobbin-costs','Package',      'finance', 3, 60, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link each leaf menu to its corresponding view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000021', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.machine.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000022', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.intermingling.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000023', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.productgrade.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000024', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.mbhead.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000025', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.mbspin.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000026', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.boxbobbincost.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN ALL 24 YARN MASTER PERMISSIONS TO SUPER_ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.yarnmaster.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
