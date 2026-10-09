-- IAM Service Database Migrations
-- 000095: Rollback Superba Cost SP menu and permissions seed (reverse dependency order).

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.superbacostsp.%'
);

DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000056';

DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_SUPERBA_COST_SP';

DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.superbacostsp.%';
