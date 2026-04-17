-- Migration: Seed Oracle Sync menu entries and permissions.
-- Description: Add menu items for Oracle Sync job management and synced data view.

BEGIN;

-- 1. Insert permissions for Oracle Sync.
INSERT INTO mst_permission (permission_code, permission_name, description, service_name, created_by)
VALUES
    ('finance.transaction.oraclesync.view',    'View Oracle Sync Jobs',     'View Oracle sync job history and status',   'finance', 'seed'),
    ('finance.transaction.oraclesync.create',  'Trigger Oracle Sync',       'Trigger manual Oracle data sync',           'finance', 'seed'),
    ('finance.transaction.oraclesync.delete',  'Cancel Oracle Sync Job',    'Cancel a queued or processing sync job',    'finance', 'seed'),
    ('finance.transaction.itemconsstockpo.view', 'View Item Cons Stock PO', 'View synced item consumption stock PO data', 'finance', 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- 2. Insert menu entries.
-- Oracle Sync page (under Finance > Transaction).
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_name, menu_url, menu_icon, module, menu_level, sort_order, is_active, is_visible, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000010', '00000000-0000-0000-0002-000000000003', 'FINANCE_ORACLE_SYNC', 'Oracle Sync', '/finance/transaction/oracle-sync', 'RefreshCw', 'finance', 3, 20, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000011', '00000000-0000-0000-0002-000000000003', 'FINANCE_ITEM_CONS_STOCK_PO', 'Item Cons Stock PO', '/finance/transaction/item-cons-stock-po', 'Database', 'finance', 3, 30, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- 3. Assign permissions to menus.
INSERT INTO menu_permissions (menu_id, permission_id, created_by)
SELECT '00000000-0000-0000-0003-000000000010', permission_id, 'seed'
FROM mst_permission WHERE permission_code LIKE 'finance.transaction.oraclesync.%'
ON CONFLICT DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, created_by)
SELECT '00000000-0000-0000-0003-000000000011', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'finance.transaction.itemconsstockpo.view'
ON CONFLICT DO NOTHING;

-- 4. Assign permissions to SUPER_ADMIN role.
INSERT INTO role_permissions (role_id, permission_id, created_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN (
    'finance.transaction.oraclesync.view',
    'finance.transaction.oraclesync.create',
    'finance.transaction.oraclesync.delete',
    'finance.transaction.itemconsstockpo.view'
  )
ON CONFLICT DO NOTHING;

COMMIT;
