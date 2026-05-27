-- Rollback: remove Employee Level menu + permissions seeds.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'iam.master.employeelevel.%'
);

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000008'
);

DELETE FROM mst_menu WHERE menu_code = 'ADMIN_EMPLOYEE_LEVEL';
DELETE FROM mst_menu WHERE menu_code = 'ADMIN_MASTER_DATA';

DELETE FROM mst_permission WHERE permission_code LIKE 'iam.master.employeelevel.%';
