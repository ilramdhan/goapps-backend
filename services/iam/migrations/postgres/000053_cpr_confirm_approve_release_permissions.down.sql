-- Revert CPR confirm/approve/release permissions.
BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'finance.product.request.confirm',
        'finance.product.request.approve',
        'finance.product.request.release'
    )
);

DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.product.request.confirm',
    'finance.product.request.approve',
    'finance.product.request.release'
);

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify', 'override',
        'review', 'reopen'
    ));

COMMIT;
