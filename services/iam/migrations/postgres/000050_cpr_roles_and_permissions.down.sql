-- Migration 000050 down: Remove CPR Roles, Permissions, and Test User Assignments

BEGIN;

-- Remove user-role assignments for CPR roles
DELETE FROM user_roles
WHERE role_id IN (SELECT role_id FROM mst_role WHERE role_code LIKE 'CPR_%');

-- Remove role permissions for CPR roles
DELETE FROM role_permissions
WHERE role_id IN (SELECT role_id FROM mst_role WHERE role_code LIKE 'CPR_%');

-- Remove new permissions from SUPER_ADMIN
DELETE FROM role_permissions
WHERE role_id = (SELECT role_id FROM mst_role WHERE role_code = 'SUPER_ADMIN')
  AND permission_id IN (
    SELECT permission_id FROM mst_permission WHERE permission_code IN (
      'finance.product.request.submit',
      'finance.product.request.review',
      'finance.product.request.reopen',
      'finance.product.route.view',
      'finance.product.route.create',
      'finance.product.route.update'
    )
  );

-- Remove CPR roles
DELETE FROM mst_role WHERE role_code LIKE 'CPR_%';

-- Remove new permission codes
DELETE FROM mst_permission WHERE permission_code IN (
  'finance.product.request.submit',
  'finance.product.request.review',
  'finance.product.request.reopen',
  'finance.product.route.view',
  'finance.product.route.create',
  'finance.product.route.update'
);

-- Revert chk_permission_action to state after migration 000049
-- (removes 'review' and 'reopen' added by this migration)
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify', 'override'
    ));

COMMIT;
