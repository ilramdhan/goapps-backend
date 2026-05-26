-- IAM Service Database Migrations
-- 000015: Seed BI (Executive Dashboard) permissions, menus, and role assignments.
--
-- Permissions follow the {service}.{module}.{entity}.{action} format. Action segment is constrained
-- to (view|create|update|delete|export|import) by chk_permission_action, so manage/trigger/commit
-- are mapped to (create|update|import) accordingly.
--
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.bi.*
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.bi.dashboard.view',   'View BI Dashboards',     'View BI dashboards in viewer (subject to per-dashboard role whitelist)', 'finance', 'bi', 'view',   true, 'seed'),
    ('finance.bi.dashboard.create', 'Create BI Dashboard',    'Create new dashboard definitions in admin panel',                      'finance', 'bi', 'create', true, 'seed'),
    ('finance.bi.dashboard.update', 'Update BI Dashboard',    'Edit existing dashboard definitions and role mappings',                'finance', 'bi', 'update', true, 'seed'),
    ('finance.bi.dashboard.delete', 'Delete BI Dashboard',    'Soft-delete dashboards',                                               'finance', 'bi', 'delete', true, 'seed'),
    ('finance.bi.datasource.view',  'View BI Data Sources',   'View bi_data_source registry',                                         'finance', 'bi', 'view',   true, 'seed'),
    ('finance.bi.datasource.update','Update BI Data Sources', 'Edit bi_data_source entries (super-admin only by default)',            'finance', 'bi', 'update', true, 'seed'),
    ('finance.bi.job.view',         'View BI ETL Jobs',       'View ETL job registry and execution logs',                             'finance', 'bi', 'view',   true, 'seed'),
    ('finance.bi.job.update',       'Trigger BI ETL Jobs',    'Manually trigger ETL jobs (maps from manage action)',                  'finance', 'bi', 'update', true, 'seed'),
    ('finance.bi.upload.create',    'Upload BI Excel',        'Upload Excel files to staging for BI fact data',                       'finance', 'bi', 'create', true, 'seed'),
    ('finance.bi.upload.import',    'Commit BI Excel',        'Commit staged Excel data into bi_fact_metric',                         'finance', 'bi', 'import', true, 'seed'),
    ('finance.bi.audit.view',       'View BI Audit Log',      'View BI configuration audit log',                                      'finance', 'bi', 'view',   true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENUS — Executive Dashboard (parent) + 3 children
-- =============================================================================

-- Level-2 parent menu — under FINANCE root (00000000-0000-0000-0001-000000000002)
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000020', '00000000-0000-0000-0001-000000000002', 'BI_PARENT', 'Executive Dashboard', NULL, 'BarChart3', 'finance', 2, 50, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- Level-3 children
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000020', '00000000-0000-0000-0002-000000000020', 'BI_VIEWER_LIST', 'Dashboards',  '/finance/bi',         'LayoutDashboard', 'finance', 3, 10, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000021', '00000000-0000-0000-0002-000000000020', 'BI_ADMIN',       'Admin Panel', '/finance/bi/admin',   'Settings',        'finance', 3, 20, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000022', '00000000-0000-0000-0002-000000000020', 'BI_UPLOAD',      'Upload Data', '/finance/bi/upload',  'Upload',          'finance', 3, 30, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link each menu to its primary permission
-- =============================================================================

-- Viewer list menu — requires dashboard.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000020', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.bi.dashboard.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Admin panel menu — requires dashboard.update
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000021', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.bi.dashboard.update' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Upload menu — requires upload.create
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000022', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.bi.upload.create' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ROLE PERMISSIONS — Assign all BI permissions to SUPER_ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.bi.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
