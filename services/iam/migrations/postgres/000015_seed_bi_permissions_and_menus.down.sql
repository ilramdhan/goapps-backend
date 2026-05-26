-- IAM Service Database Migrations
-- 000015 (DOWN): Remove BI permissions, menus, and role assignments.

BEGIN;

-- Remove role-permission links for BI perms
DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'finance.bi.%');

-- Remove menu-permission links for BI menus
DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000020',
    '00000000-0000-0000-0003-000000000021',
    '00000000-0000-0000-0003-000000000022'
);

-- Remove BI menus (children first to respect FK)
DELETE FROM mst_menu WHERE menu_code IN ('BI_VIEWER_LIST', 'BI_ADMIN', 'BI_UPLOAD');
DELETE FROM mst_menu WHERE menu_code = 'BI_PARENT';

-- Remove BI permissions
DELETE FROM mst_permission WHERE permission_code LIKE 'finance.bi.%';

COMMIT;
