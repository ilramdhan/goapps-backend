-- Remove Employee Group menu permissions linkage
DELETE FROM menu_permissions WHERE menu_id = '00000000-0000-0000-0003-000000000009';

-- Remove Employee Group permissions from SUPER_ADMIN role
DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'iam.master.employeegroup.%');

-- Remove Employee Group permissions
DELETE FROM mst_permission WHERE permission_code LIKE 'iam.master.employeegroup.%';

-- Remove Employee Group menu item
DELETE FROM mst_menu WHERE menu_code = 'ADMIN_EMPLOYEE_GROUP';
