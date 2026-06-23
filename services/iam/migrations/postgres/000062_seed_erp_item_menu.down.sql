-- Remove SUPER_ADMIN role assignments for ERP Item permissions.
DELETE FROM role_permissions rp
USING mst_permission p
WHERE rp.permission_id = p.permission_id
  AND p.permission_code IN (
    'finance.master.erpitem.view',
    'finance.master.erpitem.create',
    'finance.master.erpitem.update',
    'finance.master.erpitem.delete'
    );

-- Remove ERP Item permissions.
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.master.erpitem.view',
    'finance.master.erpitem.create',
    'finance.master.erpitem.update',
    'finance.master.erpitem.delete'
    );

-- Remove ERP Item menu entry.
DELETE FROM mst_menu WHERE menu_code = 'FINANCE_ERP_ITEM';
