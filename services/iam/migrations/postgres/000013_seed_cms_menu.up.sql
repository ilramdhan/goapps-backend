-- IAM Service Database Migrations
-- 000013: Seed CMS menu and permissions
--
-- Adds CMS Management menu under Administrator in the sidebar navigation.
-- Seeds iam.cms.{page,section,setting}.* permissions for RBAC.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — iam.cms.page.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.cms.page.view',   'View CMS Pages',   'View CMS pages list and details',   'iam', 'cms', 'view',   true, 'seed'),
    ('iam.cms.page.create', 'Create CMS Page',   'Create new CMS pages',              'iam', 'cms', 'create', true, 'seed'),
    ('iam.cms.page.update', 'Update CMS Page',   'Update existing CMS pages',         'iam', 'cms', 'update', true, 'seed'),
    ('iam.cms.page.delete', 'Delete CMS Page',   'Delete CMS pages',                  'iam', 'cms', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- PERMISSIONS — iam.cms.section.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.cms.section.view',   'View CMS Sections',   'View CMS sections list and details',   'iam', 'cms', 'view',   true, 'seed'),
    ('iam.cms.section.create', 'Create CMS Section',   'Create new CMS sections',              'iam', 'cms', 'create', true, 'seed'),
    ('iam.cms.section.update', 'Update CMS Section',   'Update existing CMS sections',         'iam', 'cms', 'update', true, 'seed'),
    ('iam.cms.section.delete', 'Delete CMS Section',   'Delete CMS sections',                  'iam', 'cms', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- PERMISSIONS — iam.cms.setting.* (no create/delete — settings are pre-seeded)
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.cms.setting.view',   'View CMS Settings',   'View CMS site settings',     'iam', 'cms', 'view',   true, 'seed'),
    ('iam.cms.setting.update', 'Update CMS Setting',   'Update CMS site settings',   'iam', 'cms', 'update', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Administrator > CMS Management
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000012', '00000000-0000-0000-0001-000000000007', 'ADMIN_CMS', 'CMS Management', '/administrator/cms', 'FileText', 'iam', 2, 35, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link CMS menu to view permissions
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000012', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.cms.page.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000012', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.cms.section.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000012', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'iam.cms.setting.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN CMS PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'iam.cms.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
