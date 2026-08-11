-- IAM Service Database Migrations
-- 000082: Rollback Spin Fixed Cost menu and permissions seed
-- FK-safe order: junction rows first, then the parents.

-- Remove role_permissions for Spin Fixed Cost
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.master.spinfixedcost.%'
);

-- Remove menu_permissions for Spin Fixed Cost
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-00000000004f';

-- Remove permissions
DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.master.spinfixedcost.%';

-- Remove menu entry
DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_SPIN_FIXED_COST';
