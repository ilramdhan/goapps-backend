-- Adds FINANCE_PRODUCT_ORDERS sidebar menu pointing to /finance/product-orders.
-- The page already exists; it just wasn't reachable from the sidebar.
-- UUID conventions follow the existing finance menu range
-- (00000000-0000-0000-0003-00000000NN), incremented past the last used slot.

-- parent_id = FINANCE_PRODUCT_COSTING (0002-...0015), the same group that
-- already houses Product Requests + Products. Originally pointed to Master
-- (0002-...0002), which placed the link in the wrong section.
INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-00000000001e',
     '00000000-0000-0000-0002-000000000015',
     'FINANCE_PRODUCT_ORDERS', 'Product Orders', '/finance/product-orders', 'ListChecks',
     'finance', 3, 22, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;
