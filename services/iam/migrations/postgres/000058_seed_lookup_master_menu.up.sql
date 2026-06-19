-- IAM Service Database Migrations
-- 000058: Seed Lookup Master menu and permissions
--
-- Adds "Lookup Master" leaf menu under FINANCE_YARN_MASTER (level-2).
-- Also seeds finance.yarnmaster.lookupmaster.{action} permissions.
--
-- UUID allocation:
--   Level-3: 00000000-0000-0000-0003-000000000038 (FINANCE_LOOKUP_MASTER)
--
-- Permission codes must match [a-z][a-z0-9]* per chk_permission_code_format.

-- =============================================================================
-- PERMISSIONS — finance.yarnmaster.lookupmaster.{action}
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.yarnmaster.lookupmaster.view',   'View Lookup Masters',  'View lookup master registry',  'finance', 'yarnmaster', 'view',   true, 'seed'),
    ('finance.yarnmaster.lookupmaster.create', 'Create Lookup Master', 'Add new master to registry',   'finance', 'yarnmaster', 'create', true, 'seed'),
    ('finance.yarnmaster.lookupmaster.delete', 'Delete Lookup Master', 'Remove master from registry',  'finance', 'yarnmaster', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU — Level-3: Lookup Master under Yarn Master
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_active, is_visible, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000038', '00000000-0000-0000-0002-000000000016',
     'FINANCE_LOOKUP_MASTER', 'Lookup Master', '/finance/yarn-master/lookup-masters',
     'Database', 'finance', 3, 70, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — bind menu to view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000038', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.yarnmaster.lookupmaster.view'
ON CONFLICT DO NOTHING;

-- =============================================================================
-- ROLE ASSIGNMENTS — assign all 3 permissions to SUPER_ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r, mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN (
      'finance.yarnmaster.lookupmaster.view',
      'finance.yarnmaster.lookupmaster.create',
      'finance.yarnmaster.lookupmaster.delete'
  )
ON CONFLICT DO NOTHING;
