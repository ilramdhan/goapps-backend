-- Migration 000049: Seed fill-assignment permissions for finance costing module.
--
-- Adds 9 permissions under finance.costing.fillconfig.* and finance.costing.filltask.*
-- plus 2 menu entries under FINANCE_PRODUCT_COSTING (0002-...0015).
--
-- Action 'override' is new — added to chk_permission_action before inserting.
-- All other actions (view/create/update/delete/approve) are already in the constraint.
--
-- Permission code format: ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- No underscores or hyphens in segments.
--
-- Level-3 UUIDs: ...0036 (Fill Config admin menu), ...0037 (Fill Tasks menu).
-- Highest previously used level-3 UUID: 00000000-0000-0000-0003-000000000035 (migration 000047).

BEGIN;

-- Extend permission action allow-list to cover 'override' (used by assignment.override).
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify', 'override'
    ));

-- =============================================================================
-- PERMISSIONS — finance.costing.fillconfig.* and finance.costing.filltask.*
-- =============================================================================

INSERT INTO mst_permission (
    permission_id, permission_code, permission_name, description,
    service_name, module_name, action_type, is_active, created_by
) VALUES
    -- Fill config (level-config management — admin only)
    (gen_random_uuid(), 'finance.costing.fillconfig.view',     'View Fill Config',           'List and view fill-assignment level configurations',       'finance', 'costing', 'view',     TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.fillconfig.create',   'Create Fill Config',         'Create fill-assignment level configurations',               'finance', 'costing', 'create',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.fillconfig.update',   'Update Fill Config',         'Update fill-assignment level configurations',               'finance', 'costing', 'update',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.fillconfig.delete',   'Delete Fill Config',         'Delete fill-assignment level configurations',               'finance', 'costing', 'delete',   TRUE, 'seed'),
    -- Fill tasks (claim/submit/approve workflow)
    (gen_random_uuid(), 'finance.costing.filltask.view',       'View Fill Tasks',            'List and view fill tasks assigned to the user or all',     'finance', 'costing', 'view',     TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.filltask.create',     'Create Fill Task',           'Create manual fill tasks (admin)',                          'finance', 'costing', 'create',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.filltask.update',     'Update Fill Task',           'Claim, submit, or approve fill tasks',                     'finance', 'costing', 'update',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.filltask.delete',     'Delete Fill Task',           'Reject or delete fill tasks',                              'finance', 'costing', 'delete',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.costing.filltask.approve',    'Approve Fill Task',          'Approve submitted fill tasks (manager/head level)',        'finance', 'costing', 'approve',  TRUE, 'seed'),
    -- Assignment override (bypass normal fill-task flow)
    (gen_random_uuid(), 'finance.costing.assignment.override', 'Override Fill Assignment',   'Override cost-fill assignment at any level (super admin)', 'finance', 'costing', 'override', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENUS — Fill Config admin + Fill Tasks under FINANCE_PRODUCT_COSTING
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000036',
     '00000000-0000-0000-0002-000000000015',
     'FINANCE_FILL_CONFIG',
     'Fill Config',
     '/finance/costing/fill-config',
     'Settings2',
     'finance', 3, 60, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000037',
     '00000000-0000-0000-0002-000000000015',
     'FINANCE_FILL_TASKS',
     'Fill Tasks',
     '/finance/costing/fill-tasks',
     'ClipboardList',
     'finance', 3, 65, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Gate each menu to its primary view permission
-- =============================================================================

-- Fill Config admin menu — requires fillconfig.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000036', p.permission_id, 'seed'
FROM mst_permission p
WHERE p.permission_code = 'finance.costing.fillconfig.view' AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Fill Tasks menu — requires filltask.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000037', p.permission_id, 'seed'
FROM mst_permission p
WHERE p.permission_code = 'finance.costing.filltask.view' AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ROLE PERMISSIONS — Assign all fill-assignment permissions to SUPER_ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'finance.costing.fill%'
  AND r.is_active = TRUE
  AND p.is_active = TRUE
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Also assign override permission to SUPER_ADMIN
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code = 'finance.costing.assignment.override'
  AND r.is_active = TRUE
  AND p.is_active = TRUE
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Assign filltask view/update/approve to FINANCE and FINANCE_MANAGER roles
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'FINANCE'
  AND p.permission_code IN (
      'finance.costing.filltask.view',
      'finance.costing.filltask.update'
  )
  AND r.is_active = TRUE
  AND p.is_active = TRUE
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code IN ('FINANCE_MANAGER', 'HEAD_FINANCE')
  AND p.permission_code IN (
      'finance.costing.filltask.view',
      'finance.costing.filltask.update',
      'finance.costing.filltask.approve',
      'finance.costing.fillconfig.view'
  )
  AND r.is_active = TRUE
  AND p.is_active = TRUE
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
