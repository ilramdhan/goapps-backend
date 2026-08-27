-- IAM Service Database Migrations
-- 000089: Rollback Shade master menu and permissions seed
--
-- Mirrors the up migration in reverse dependency order.
-- No DDL was added by the up migration, so no constraint is restored here.

-- 1. Remove role assignments
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.shade.%'
);

-- 2. Remove menu <-> permission links
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000051';

-- 3. Remove the sidebar entry
DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_SHADE';

-- 4. Remove the permissions themselves
--    (menu_id backfilled in step 4 of the up migration disappears with the row.)
DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.shade.%';
