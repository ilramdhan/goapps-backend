-- Rollback: remove workflow transition permissions and revert action_type constraint.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'iam.master.employeelevel.submit',
        'iam.master.employeelevel.approve',
        'iam.master.employeelevel.release',
        'iam.master.employeelevel.bypass'
    )
);

DELETE FROM mst_permission
WHERE permission_code IN (
    'iam.master.employeelevel.submit',
    'iam.master.employeelevel.approve',
    'iam.master.employeelevel.release',
    'iam.master.employeelevel.bypass'
);

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN ('view', 'create', 'update', 'delete', 'export', 'import'));
