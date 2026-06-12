-- Migration 000053: Add CPR confirm/approve/release permission codes and assign to roles.
--
-- New permission codes:
--   finance.product.request.confirm  — transition PARAMETER_COMPLETE → CONFIRMED
--   finance.product.request.approve  — transition CONFIRMED → APPROVED
--   finance.product.request.release  — transition APPROVED → RELEASED
--
-- Assignment:
--   finance.product.request.confirm → CPR_REVIEWER + CPR_ADMIN (finance01, financemgr)
--   finance.product.request.approve → CPR_APPROVER + CPR_ADMIN (productionmgr)
--   finance.product.request.release → CPR_ADMIN (financemgr)

BEGIN;

-- 1. Extend chk_permission_action to include 'confirm' (approve/release already present).
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify', 'override',
        'review', 'reopen', 'confirm'
    ));

-- 2. Insert new permission codes.
INSERT INTO mst_permission (
    permission_id, permission_code, permission_name, description,
    service_name, module_name, action_type, is_active, created_by
) VALUES
    (gen_random_uuid(), 'finance.product.request.confirm',
     'Confirm Product Request',
     'Transition product request from PARAMETER_COMPLETE to CONFIRMED',
     'finance', 'product', 'confirm', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.request.approve',
     'Approve Product Request',
     'Transition product request from CONFIRMED to APPROVED',
     'finance', 'product', 'approve', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.request.release',
     'Release Product Request',
     'Transition product request from APPROVED to RELEASED (locks for costing)',
     'finance', 'product', 'release', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- 3. Assign confirm permission to CPR_REVIEWER and CPR_ADMIN.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code IN ('CPR_REVIEWER', 'CPR_ADMIN')
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code = 'finance.product.request.confirm'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 4. Assign approve permission to CPR_APPROVER and CPR_ADMIN.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code IN ('CPR_APPROVER', 'CPR_ADMIN')
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code = 'finance.product.request.approve'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 5. Assign release permission to CPR_ADMIN only (financemgr has CPR_ADMIN).
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_ADMIN'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code = 'finance.product.request.release'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 6. Assign all three new permissions to SUPER_ADMIN.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.confirm',
    'finance.product.request.approve',
    'finance.product.request.release'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
