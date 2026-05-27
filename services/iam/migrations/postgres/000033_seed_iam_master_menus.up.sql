-- IAM Service Database Migrations
-- 000033: Seed IAM master-data menus under ADMIN_MASTER_DATA parent.
--
-- Adds 5 level-3 menus pointing at:
--   /administrator/master/companies          (ADMIN_MASTER_COMPANY)
--   /administrator/master/divisions          (ADMIN_MASTER_DIVISION)
--   /administrator/master/departments        (ADMIN_MASTER_DEPARTMENT)
--   /administrator/master/sections           (ADMIN_MASTER_SECTION)
--   /administrator/master/company-mappings   (ADMIN_MASTER_COMPANY_MAPPING)
--
-- Each menu is gated by its corresponding iam.master.<entity>.view permission.

-- =============================================================================
-- LEVEL 3 — Master entries under ADMIN_MASTER_DATA (00...0002-...000013)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by) VALUES
    ('00000000-0000-0000-0003-000000000017', '00000000-0000-0000-0002-000000000013', 'ADMIN_MASTER_COMPANY',         'Companies',        '/administrator/master/companies',        'Building2', 'iam', 3, 50, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000018', '00000000-0000-0000-0002-000000000013', 'ADMIN_MASTER_DIVISION',        'Divisions',        '/administrator/master/divisions',        'Network',   'iam', 3, 51, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000019', '00000000-0000-0000-0002-000000000013', 'ADMIN_MASTER_DEPARTMENT',      'Departments',      '/administrator/master/departments',      'Building',  'iam', 3, 52, true, true, 'seed'),
    ('00000000-0000-0000-0003-00000000001a', '00000000-0000-0000-0002-000000000013', 'ADMIN_MASTER_SECTION',         'Sections',         '/administrator/master/sections',         'Users',     'iam', 3, 53, true, true, 'seed'),
    ('00000000-0000-0000-0003-00000000001b', '00000000-0000-0000-0002-000000000013', 'ADMIN_MASTER_COMPANY_MAPPING', 'Company Mappings', '/administrator/master/company-mappings', 'MapPin',    'iam', 3, 54, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — gate each menu by its <entity>.view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000017', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.company.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000018', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.division.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000019', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.department.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-00000000001a', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.section.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-00000000001b', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.companymapping.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;
