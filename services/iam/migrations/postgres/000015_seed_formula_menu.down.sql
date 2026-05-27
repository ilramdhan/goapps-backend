-- Rollback: Remove Formula menu and permissions

-- Remove menu permissions link
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000006';

-- Remove role permissions for Formula
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.formula.%'
);

-- Remove menu entry
DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_FORMULA';

-- Remove permissions
DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.formula.%';
