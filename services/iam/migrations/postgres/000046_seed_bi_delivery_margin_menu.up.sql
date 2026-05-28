-- Seed: BI Delivery Margin viewer menu + permissions.
-- Parent = BI_PARENT (level-2 UUID 00000000-0000-0000-0002-000000000020 from migration 000044).
-- Level-3 UUID 00000000-0000-0000-0003-000000000033 is the next available slot after ...032.
BEGIN;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000033',
     '00000000-0000-0000-0002-000000000020',
     'BI_DELIVERY_MARGIN', 'Delivery Margin', '/finance/bi/DELIVERY_MARGIN', 'TrendingUp',
     'finance', 3, 34, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.bi.deliverymargin.view',   'View Delivery Margin Dashboard',   'View delivery margin dashboard',    'finance', 'bi', 'view',   true, 'seed'),
    ('finance.bi.deliverymargin.export', 'Export Delivery Margin Dashboard', 'Export delivery margin chart data', 'finance', 'bi', 'export', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code IN ('finance.bi.deliverymargin.view', 'finance.bi.deliverymargin.export')
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
