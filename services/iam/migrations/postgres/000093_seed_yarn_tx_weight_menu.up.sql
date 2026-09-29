-- IAM Service Database Migrations
-- 000093: Seed TX Weight (Yarn TX Weight master) menu and permissions
--
-- Adds TX Weight as a child of Finance > Master in the sidebar navigation and
-- seeds the finance.master.yarntxweight.* permissions guarding the finance
-- YarnTxWeightService RPCs (finance auth_interceptor.go).
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   so the entity segment is the concatenated run `yarntxweight`.

-- =============================================================================
-- PERMISSIONS — finance.master.yarntxweight.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.yarntxweight.view',   'View TX Weight',   'View TX weight rules list and details', 'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.yarntxweight.create', 'Create TX Weight', 'Create new TX weight rules',            'finance', 'master', 'create', true, 'seed'),
    ('finance.master.yarntxweight.update', 'Update TX Weight', 'Update existing TX weight rules',       'finance', 'master', 'update', true, 'seed'),
    ('finance.master.yarntxweight.delete', 'Delete TX Weight', 'Delete TX weight rules',                'finance', 'master', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > TX Weight
-- =============================================================================
-- Highest occupied level-3 seq across all migrations is ...0051, so ...0052 is
-- the next free one. Guarded on both menu_id and menu_code (see 000082).

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
SELECT '00000000-0000-0000-0003-000000000052', '00000000-0000-0000-0002-000000000002', 'FINANCE_YARN_TX_WEIGHT', 'TX Weight', '/finance/master/yarn-tx-weight', 'Scale', 'finance', 3, 50, true, true, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_menu
    WHERE menu_id = '00000000-0000-0000-0003-000000000052'
       OR menu_code = 'FINANCE_YARN_TX_WEIGHT'
);

-- =============================================================================
-- MENU PERMISSIONS — Link TX Weight menu to its view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_YARN_TX_WEIGHT'
    AND p.permission_code = 'finance.master.yarntxweight.view'
    AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN TX WEIGHT PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.yarntxweight.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
