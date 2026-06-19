-- Revert: remove Lookup Master menu + permissions seeded in 000058.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.yarnmaster.lookupmaster.%'
);
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000038';
DELETE FROM mst_menu WHERE menu_code = 'FINANCE_LOOKUP_MASTER';
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.yarnmaster.lookupmaster.view',
    'finance.yarnmaster.lookupmaster.create',
    'finance.yarnmaster.lookupmaster.delete'
);
