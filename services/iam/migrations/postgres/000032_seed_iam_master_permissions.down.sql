-- IAM Service Database Migrations
-- 000032 (down): Remove seeded master & user-mapping permissions and revert
-- chk_permission_action whitelist (drop 'remove').

DELETE FROM role_permissions
USING mst_permission p
WHERE role_permissions.permission_id = p.permission_id
  AND (
        p.permission_code LIKE 'iam.master.company.%'
     OR p.permission_code LIKE 'iam.master.division.%'
     OR p.permission_code LIKE 'iam.master.department.%'
     OR p.permission_code LIKE 'iam.master.section.%'
     OR p.permission_code LIKE 'iam.master.companymapping.%'
     OR p.permission_code LIKE 'iam.user.companymapping.%'
  );

DELETE FROM mst_permission
WHERE permission_code LIKE 'iam.master.company.%'
   OR permission_code LIKE 'iam.master.division.%'
   OR permission_code LIKE 'iam.master.department.%'
   OR permission_code LIKE 'iam.master.section.%'
   OR permission_code LIKE 'iam.master.companymapping.%'
   OR permission_code LIKE 'iam.user.companymapping.%';

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate',
        'assign', 'resolve', 'reject', 'duplicate'
    ));
