-- Reverse of 000079: remove PPC role/permission/menu seeds.

-- 1. role_permissions for PPC permissions
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'ppc.%'
);

-- 2. menu_permissions for PPC menus
DELETE FROM menu_permissions WHERE menu_id IN (
  SELECT menu_id FROM mst_menu WHERE menu_code LIKE 'PPC%'
);

-- 3. menus (leaves + sections + root) — children first via level ordering
DELETE FROM mst_menu WHERE menu_code IN (
  'PPC_MASTER_MACHINE','PPC_MASTER_MACHINE_GROUP','PPC_MASTER_LOT','PPC_MASTER_PRODUCT_CONFIG',
  'PPC_MASTER_MACHINE_PARAMETER','PPC_MASTER_THRESHOLD','PPC_MASTER_DOWNTIME_REASON','PPC_MASTER_WASTE_CATEGORY',
  'PPC_DEMAND','PPC_PLAN','PPC_WORK_ORDERS','PPC_DAILY_PERFORMANCE','PPC_DASHBOARDS','PPC_MASTERS',
  'PPC'
);

-- 4. permissions
DELETE FROM mst_permission WHERE permission_code LIKE 'ppc.%';

-- 5. roles (only PPC-introduced roles)
DELETE FROM mst_role WHERE role_code IN ('PPC','PC','PM','MARKETING','MANAGEMENT','OPERATOR');
