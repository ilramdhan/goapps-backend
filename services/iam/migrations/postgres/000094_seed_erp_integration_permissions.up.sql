-- IAM Service Database Migrations
-- 000094: Seed ERP cost integration permissions (finance.cost.erpintegration.*,
--         finance.cost.erprule.*), 3 menus, 2 roles.
--
-- Menu UUIDs 0053-0055 (main already uses 0052 via 000093).
-- chk_permission_action widened from 40 (000092) to 43 values: + push, valuate, restore.
-- valuate and restore are deliberately granted to NO role (asserted at the end).

-- 0. EXTEND chk_permission_action
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke','bulkunvalidate','bulksubmit','bulkvalidate',
    'push','valuate','restore'
  ));

-- 1. PERMISSIONS
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    (gen_random_uuid(), 'finance.cost.erpintegration.view', 'View ERP Integration', 'View on ERP integration', 'finance', 'cost', 'view', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.trigger', 'Trigger ERP Integration', 'Trigger on ERP integration', 'finance', 'cost', 'trigger', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.validate', 'Validate ERP Integration', 'Validate on ERP integration', 'finance', 'cost', 'validate', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.approve', 'Approve ERP Integration', 'Approve on ERP integration', 'finance', 'cost', 'approve', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.push', 'Push ERP Integration', 'Push on ERP integration', 'finance', 'cost', 'push', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.valuate', 'Valuate ERP Integration', 'Valuate on ERP integration', 'finance', 'cost', 'valuate', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.restore', 'Restore ERP Integration', 'Restore on ERP integration', 'finance', 'cost', 'restore', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.lock', 'Lock ERP Integration', 'Lock on ERP integration', 'finance', 'cost', 'lock', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.unlock', 'Unlock ERP Integration', 'Unlock on ERP integration', 'finance', 'cost', 'unlock', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.update', 'Update ERP Integration', 'Update on ERP integration', 'finance', 'cost', 'update', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.sync', 'Sync ERP Integration', 'Sync on ERP integration', 'finance', 'cost', 'sync', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erpintegration.export', 'Export ERP Integration', 'Export on ERP integration', 'finance', 'cost', 'export', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erprule.view', 'View ERP Rule', 'View on ERP rule', 'finance', 'cost', 'view', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erprule.create', 'Create ERP Rule', 'Create on ERP rule', 'finance', 'cost', 'create', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erprule.update', 'Update ERP Rule', 'Update on ERP rule', 'finance', 'cost', 'update', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erprule.delete', 'Delete ERP Rule', 'Delete on ERP rule', 'finance', 'cost', 'delete', true, 'seed'),
    (gen_random_uuid(), 'finance.cost.erprule.export', 'Export ERP Rule', 'Export on ERP rule', 'finance', 'cost', 'export', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- 2. MENUS
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000053', '00000000-0000-0000-0002-000000000015', 'FINANCE_ERP_INTEGRATION', 'ERP Integration', '/finance/erp-integration', 'ArrowLeftRight', 'finance', 3, 25, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000054', '00000000-0000-0000-0002-000000000015', 'FINANCE_ERP_INTEGRATION_DETAIL', 'ERP Integration Detail', '/finance/erp-integration/[id]', NULL, 'finance', 3, 26, false, true, 'seed'),
    ('00000000-0000-0000-0003-000000000055', '00000000-0000-0000-0002-000000000002', 'FINANCE_ERP_RULES', 'ERP Rules', '/finance/master/erp-rules', 'Scale', 'finance', 3, 46, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- 3. MENU PERMISSIONS (view gates)
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m JOIN mst_permission p ON p.is_active = TRUE
WHERE (m.menu_code IN ('FINANCE_ERP_INTEGRATION','FINANCE_ERP_INTEGRATION_DETAIL') AND p.permission_code = 'finance.cost.erpintegration.view')
   OR (m.menu_code = 'FINANCE_ERP_RULES' AND p.permission_code = 'finance.cost.erprule.view')
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- 4. BACKFILL permission.menu_id (convention of 000066)
UPDATE mst_permission p SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'FINANCE_ERP_INTEGRATION' AND p.permission_code LIKE 'finance.cost.erpintegration.%' AND p.menu_id IS NULL;
UPDATE mst_permission p SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'FINANCE_ERP_RULES' AND p.permission_code LIKE 'finance.cost.erprule.%' AND p.menu_id IS NULL;

-- 5. ROLES
INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by) VALUES
  (gen_random_uuid(), 'ERP_COSTING','ERP Costing','Runs and validates ERP cost integration batches',FALSE,TRUE,'seed'),
  (gen_random_uuid(), 'ERP_FIN_APPROVER','ERP Finance Approver','Approves and pushes ERP cost integration batches; manages ERP rules',FALSE,TRUE,'seed')
ON CONFLICT (role_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'ERP_COSTING' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.cost.erpintegration.view',
  'finance.cost.erpintegration.trigger',
  'finance.cost.erpintegration.validate',
  'finance.cost.erpintegration.lock',
  'finance.cost.erpintegration.update',
  'finance.cost.erpintegration.sync',
  'finance.cost.erpintegration.export',
  'finance.cost.erprule.view'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'ERP_FIN_APPROVER' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.cost.erpintegration.view',
  'finance.cost.erpintegration.approve',
  'finance.cost.erpintegration.push',
  'finance.cost.erpintegration.export',
  'finance.cost.erprule.view',
  'finance.cost.erprule.create',
  'finance.cost.erprule.update',
  'finance.cost.erprule.delete',
  'finance.cost.erprule.export'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- SUPER_ADMIN: everything except valuate/restore
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed' FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = TRUE AND p.is_active = TRUE AND p.permission_code IN (
  'finance.cost.erpintegration.view',
  'finance.cost.erpintegration.trigger',
  'finance.cost.erpintegration.validate',
  'finance.cost.erpintegration.approve',
  'finance.cost.erpintegration.push',
  'finance.cost.erpintegration.lock',
  'finance.cost.erpintegration.unlock',
  'finance.cost.erpintegration.update',
  'finance.cost.erpintegration.sync',
  'finance.cost.erpintegration.export',
  'finance.cost.erprule.view',
  'finance.cost.erprule.create',
  'finance.cost.erprule.update',
  'finance.cost.erprule.delete',
  'finance.cost.erprule.export'
)
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 6. GUARD: valuate/restore must have zero grants
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM role_permissions rp JOIN mst_permission p ON p.permission_id = rp.permission_id
             WHERE p.permission_code IN ('finance.cost.erpintegration.valuate','finance.cost.erpintegration.restore')) THEN
    RAISE EXCEPTION 'valuate/restore permissions must not be granted to any role';
  END IF;
END $$;
