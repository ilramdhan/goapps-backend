-- 000099 down: restore the MB role grants exactly as 000068/000083/000085/000087 left them.
--
-- ⚠ Dropping MB_ADMIN cascades to user_roles (fk ON DELETE CASCADE, 000004:81):
-- users assigned MB_ADMIN lose that role assignment.
-- finance.module.root.view is NOT deleted — it belongs to seeds/main.go.

-- 1. Re-grant the dead-feature permissions revoked by the up migration
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM (VALUES
    ('MB_APPROVER',  'finance.mb.head.unapprove'),
    ('MB_APPROVER',  'finance.mb.recipe.lock'),
    ('MB_APPROVER',  'finance.yarnmaster.mbspin.duplicate'),
    ('MB_VALIDATOR', 'finance.mb.head.revoke'),
    ('MB_VALIDATOR', 'finance.mb.head.validate'),
    ('MB_VALIDATOR', 'finance.mb.recipe.lock'),
    ('MB_VALIDATOR', 'finance.yarnmaster.mbspin.duplicate')
) AS x(role_code, permission_code)
JOIN mst_role r ON r.role_code = x.role_code
JOIN mst_permission p ON p.permission_code = x.permission_code
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 2. Remove the grants the up migration ADDED to the three existing roles
--    (only the ones they did not hold before 000099).
DELETE FROM role_permissions rp
USING mst_role r, mst_permission p
WHERE rp.role_id = r.role_id
  AND rp.permission_id = p.permission_id
  AND r.role_code IN ('MB_DRAFTER', 'MB_APPROVER', 'MB_VALIDATOR')
  AND p.permission_code IN (
      'finance.module.root.view',
      'finance.master.mbxsection.view',
      'finance.master.mbxsectionfactor.view',
      'finance.yarnmaster.mbspin.view',
      'finance.cost.result.view',
      'finance.cost.history.view'
  );

DELETE FROM role_permissions rp
USING mst_role r, mst_permission p
WHERE rp.role_id = r.role_id
  AND rp.permission_id = p.permission_id
  AND r.role_code = 'MB_DRAFTER'
  AND p.permission_code = 'finance.mb.head.delete';

-- 3. Drop MB_ADMIN (role_permissions + user_roles cascade)
DELETE FROM mst_role WHERE role_code = 'MB_ADMIN';
