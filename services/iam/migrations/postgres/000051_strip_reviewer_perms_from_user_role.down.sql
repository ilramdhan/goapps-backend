-- Revert: restore CPR reviewer perms to USER role

BEGIN;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'USER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.reject',
    'finance.product.request.resolve',
    'finance.product.request.assign'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
