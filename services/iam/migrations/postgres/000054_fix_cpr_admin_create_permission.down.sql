-- Restore finance.product.request.create for CPR_ADMIN (reverting migration 000054).
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r, mst_permission p
WHERE r.role_code = 'CPR_ADMIN'
  AND p.permission_code = 'finance.product.request.create'
ON CONFLICT (role_id, permission_id) DO NOTHING;
