-- IAM Service Database Migrations
-- 000093: Rollback TX Weight menu and permissions seed
-- FK-safe order: junction rows first, then the parents.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.yarntxweight.%'
);

DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000052';

DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.yarntxweight.%';

DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_YARN_TX_WEIGHT';
