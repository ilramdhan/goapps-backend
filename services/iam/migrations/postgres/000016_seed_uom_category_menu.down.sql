-- IAM Service Database Migrations
-- 000016: Rollback UOM Category menu and permissions seed

-- Remove role_permissions for UOM Category
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.uomcategory.%'
);

-- Remove menu_permissions for UOM Category
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000007';

-- Remove menu entry
DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_UOM_CATEGORY';

-- Remove permissions
DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.uomcategory.%';
