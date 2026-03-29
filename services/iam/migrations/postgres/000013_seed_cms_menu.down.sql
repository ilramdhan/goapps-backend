-- Rollback: Remove CMS menu and permissions

-- Remove menu permissions link
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0002-000000000012';

-- Remove role permissions for CMS
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'iam.cms.%'
);

-- Remove menu entry
DELETE FROM mst_menu
WHERE menu_code = 'ADMIN_CMS';

-- Remove permissions
DELETE FROM mst_permission
WHERE permission_code LIKE 'iam.cms.%';
