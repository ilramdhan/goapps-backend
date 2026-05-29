-- Add per-dashboard sidebar entries for EBITDA and NET_PROFIT under Executive Dashboard.
-- DELIVERY_MARGIN (000000000033) already exists from migration 000046.
-- Parent: BI_PARENT (00000000-0000-0000-0002-000000000020) from migration 000044.
BEGIN;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000034',
     '00000000-0000-0000-0002-000000000020',
     'BI_EBITDA', 'EBITDA Performance', '/finance/bi/EBITDA', 'BarChart2',
     'finance', 3, 31, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000035',
     '00000000-0000-0000-0002-000000000020',
     'BI_NET_PROFIT', 'Net Profit Trend', '/finance/bi/NET_PROFIT', 'TrendingUp',
     'finance', 3, 32, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

COMMIT;
