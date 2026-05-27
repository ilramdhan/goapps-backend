-- IAM Service Database Migrations
-- 000029: Seed Product Costing menus and link to permissions.
--
-- Adds "Product Costing" parent menu under Finance (level 2), with two children:
--   - Product Requests  → /finance/product-requests
--   - Products          → /finance/products
--
-- Links each child menu to its 'view' permission so visibility is permission-gated.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- LEVEL 2 — Finance > Product Costing (parent group menu)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0002-000000000015', '00000000-0000-0000-0001-000000000002', 'FINANCE_PRODUCT_COSTING',
     'Product Costing', NULL, 'Boxes', 'finance', 2, 90, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- LEVEL 3 — Product Requests + Products (leaf pages under Product Costing)
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000015', '00000000-0000-0000-0002-000000000015', 'FINANCE_PRODUCT_REQUESTS',
     'Product Requests', '/finance/product-requests', 'Inbox', 'finance', 3, 10, true, true, 'seed'),

    ('00000000-0000-0000-0003-000000000016', '00000000-0000-0000-0002-000000000015', 'FINANCE_PRODUCTS',
     'Products', '/finance/products', 'Package', 'finance', 3, 20, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — link each child menu to its 'view' permission
-- (parent menu has no permission entry → visible to all authenticated users)
-- =============================================================================

-- Product Requests menu — visible to users with request.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000015', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.product.request.view'
  AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Products menu — visible to users with master.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000016', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.product.master.view'
  AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;
