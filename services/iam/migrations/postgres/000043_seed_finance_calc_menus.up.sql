-- Phase C (Calc Engine) S8a.8: seed sidebar menus for Calc Jobs + Cost Results.
--
-- Parent menu: FINANCE_PRODUCT_COSTING (level 2, uuid 00000000-0000-0000-0002-000000000015).
-- The plan referenced a FINANCE_COST parent, but that menu does not exist in
-- this codebase; FINANCE_PRODUCT_COSTING is the level-2 home for cost features
-- (already contains Product Requests, Products, Product Routes, Product Orders).
--
-- Level-3 UUIDs picked: 00000000-0000-0000-0003-00000000001f and ...0020
-- (next available after the last used slot ...001e = FINANCE_PRODUCT_ORDERS).
-- Sort orders 23/24 follow the existing FINANCE_PRODUCT_ORDERS (22).

BEGIN;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-00000000001f',
     '00000000-0000-0000-0002-000000000015',
     'FINANCE_CALC_JOBS', 'Calc Jobs', '/finance/calc-jobs', 'Calculator',
     'finance', 3, 23, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000020',
     '00000000-0000-0000-0002-000000000015',
     'FINANCE_COST_RESULTS', 'Cost Results', '/finance/cost-results', 'TrendingUp',
     'finance', 3, 24, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- Menu permission gating: only users with the matching .view permission see these.
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_CALC_JOBS'
  AND p.permission_code = 'finance.cost.caljob.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_COST_RESULTS'
  AND p.permission_code = 'finance.cost.result.view'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

COMMIT;
