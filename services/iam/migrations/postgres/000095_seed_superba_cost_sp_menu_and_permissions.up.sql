-- IAM Service Database Migrations
-- 000095: Seed Superba Cost SP master menu (Finance > Master) and its
--         finance.master.superbacostsp.* permissions.
--
-- Mirrors 000089 (Shade). Page: /finance/master/superba-cost-sps (frontend plan P8-T3);
-- RPCs: SuperbaCostSpService (Create/Get/Update/Delete/List/Sync).
--
-- Placement facts (verified by grep over *.up.sql):
--   highest menu UUID seeded is ...0055 (000094) => ...0056 is free.
--   Siblings under FINANCE_MASTER (0002-...02): Shade 45, ERP Rules 46, TX Weight 50.
--   sort_order 50 is already taken by TX Weight, so 55 is used to land last in the group.
-- action_type values view/create/update/delete/sync are all in chk_permission_action
-- (000094), so this migration contains ZERO DDL. permission_code format
-- {service}.{module}.{entity}.{action} satisfies chk_permission_code_format.
-- All statements are idempotent.

-- 1. PERMISSIONS
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    (gen_random_uuid(), 'finance.master.superbacostsp.view',   'View Superba Cost SP',   'View Superba Cost SP master list and details',              'finance', 'master', 'view',   true, 'seed'),
    (gen_random_uuid(), 'finance.master.superbacostsp.create', 'Create Superba Cost SP', 'Create a new Superba Cost SP row',                          'finance', 'master', 'create', true, 'seed'),
    (gen_random_uuid(), 'finance.master.superbacostsp.update', 'Update Superba Cost SP', 'Update an existing Superba Cost SP row',                    'finance', 'master', 'update', true, 'seed'),
    (gen_random_uuid(), 'finance.master.superbacostsp.delete', 'Delete Superba Cost SP', 'Delete (soft) a Superba Cost SP row',                       'finance', 'master', 'delete', true, 'seed'),
    (gen_random_uuid(), 'finance.master.superbacostsp.sync',   'Sync Superba Cost SP',   'Sync Superba Cost SP master from Oracle legacy (read-only)', 'finance', 'master', 'sync',   true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- 2. MENU ENTRY — Finance > Master > Superba Cost SP
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
SELECT '00000000-0000-0000-0003-000000000056', '00000000-0000-0000-0002-000000000002', 'FINANCE_SUPERBA_COST_SP', 'Superba Cost SP', '/finance/master/superba-cost-sps', 'Palette', 'finance', 3, 55, true, true, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_menu
    WHERE menu_id = '00000000-0000-0000-0003-000000000056'
       OR menu_code = 'FINANCE_SUPERBA_COST_SP'
);

-- 3. MENU PERMISSIONS — view gate (no rows would make the menu world-visible)
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_SUPERBA_COST_SP'
  AND p.permission_code = 'finance.master.superbacostsp.view'
  AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- 4. BACKFILL permission.menu_id (convention of 000066)
UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'FINANCE_SUPERBA_COST_SP'
  AND p.permission_code LIKE 'finance.master.superbacostsp.%'
  AND p.menu_id IS NULL;

-- 5. GRANT ALL TO SUPER_ADMIN (same as 000089; 000090 only covered permissions existing at that time)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'finance.master.superbacostsp.%'
  AND r.is_active = true
  AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
