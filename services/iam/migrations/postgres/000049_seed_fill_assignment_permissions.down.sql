-- Migration 000049 DOWN: Remove fill-assignment permissions and menus.

BEGIN;

-- Remove menu permissions
DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000036',
    '00000000-0000-0000-0003-000000000037'
);

-- Remove menus
DELETE FROM mst_menu
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000036',
    '00000000-0000-0000-0003-000000000037'
);

-- Remove role_permissions for fill-assignment permissions
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.costing.fillconfig.%'
       OR permission_code LIKE 'finance.costing.filltask.%'
       OR permission_code = 'finance.costing.assignment.override'
);

-- Remove permissions
DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.costing.fillconfig.%'
   OR permission_code LIKE 'finance.costing.filltask.%'
   OR permission_code = 'finance.costing.assignment.override';

-- Restore chk_permission_action to the state before 000049 (without 'override').
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify'
    ));

COMMIT;
