-- IAM Service Database Migrations
-- 000025: Seed Oracle Sync menu entries and permissions
--
-- Adds "Oracle Sync" and "Item Cons Stock PO" under Finance > Transaction.
-- Seeds finance.transaction.oraclesync.* and finance.transaction.itemconsstockpo.view permissions.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- PERMISSIONS — finance.transaction.oraclesync.* + itemconsstockpo.view
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.transaction.oraclesync.view',       'View Oracle Sync Jobs',     'View Oracle sync job history and status',    'finance', 'transaction', 'view',   true, 'seed'),
    ('finance.transaction.oraclesync.create',     'Trigger Oracle Sync',       'Trigger manual Oracle data sync',            'finance', 'transaction', 'create', true, 'seed'),
    ('finance.transaction.oraclesync.delete',     'Cancel Oracle Sync Job',    'Cancel a queued or processing sync job',     'finance', 'transaction', 'delete', true, 'seed'),
    ('finance.transaction.itemconsstockpo.view',  'View Item Cons Stock PO',   'View synced item consumption stock PO data', 'finance', 'transaction', 'view',   true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENUS — Finance > Transaction > Oracle Sync / Item Cons Stock PO (Level 3 leaves)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000010', '00000000-0000-0000-0002-000000000003', 'FINANCE_ORACLE_SYNC', 'Oracle Sync', '/finance/transaction/oracle-sync', 'RefreshCw', 'finance', 3, 20, true, true, 'seed'),
    ('00000000-0000-0000-0003-000000000011', '00000000-0000-0000-0002-000000000003', 'FINANCE_ITEM_CONS_STOCK_PO', 'Item Cons Stock PO', '/finance/transaction/item-cons-stock-po', 'Database', 'finance', 3, 30, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link menus to view permissions
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000010', permission_id, 'seed'
FROM mst_permission
WHERE permission_code LIKE 'finance.transaction.oraclesync.%' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000011', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.transaction.itemconsstockpo.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN ORACLE SYNC PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
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
  AND r.is_active = true
  AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
