-- Rollback: Remove Oracle Sync menu entries and permissions.

BEGIN;

-- Remove role-permission assignments.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'finance.transaction.oraclesync.view',
        'finance.transaction.oraclesync.create',
        'finance.transaction.oraclesync.delete',
        'finance.transaction.itemconsstockpo.view'
    )
);

-- Remove menu-permission assignments.
DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000010',
    '00000000-0000-0000-0003-000000000011'
);

-- Remove menus.
DELETE FROM mst_menu
WHERE menu_code IN ('FINANCE_ORACLE_SYNC', 'FINANCE_ITEM_CONS_STOCK_PO');

-- Remove permissions.
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.transaction.oraclesync.view',
    'finance.transaction.oraclesync.create',
    'finance.transaction.oraclesync.delete',
    'finance.transaction.itemconsstockpo.view'
);

COMMIT;
