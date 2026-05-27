-- Remove v1.3 permissions + role grants. Revert chk_permission_action whitelist to 000032 state.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'iam.master.workflowtemplate.%'
       OR permission_code LIKE 'finance.transaction.workflow.%'
       OR permission_code LIKE 'finance.master.producttype.%'
       OR permission_code LIKE 'finance.transaction.prdrequest.%'
       OR permission_code LIKE 'finance.transaction.cstproduct.%'
       OR permission_code LIKE 'finance.transaction.chat.%'
       OR permission_code = 'finance.transaction.costcalc.trigger'
);

DELETE FROM mst_permission
WHERE permission_code LIKE 'iam.master.workflowtemplate.%'
   OR permission_code LIKE 'finance.transaction.workflow.%'
   OR permission_code LIKE 'finance.master.producttype.%'
   OR permission_code LIKE 'finance.transaction.prdrequest.%'
   OR permission_code LIKE 'finance.transaction.cstproduct.%'
   OR permission_code LIKE 'finance.transaction.chat.%'
   OR permission_code = 'finance.transaction.costcalc.trigger';

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate',
        'assign', 'resolve', 'reject', 'duplicate',
        'remove'
    ));
