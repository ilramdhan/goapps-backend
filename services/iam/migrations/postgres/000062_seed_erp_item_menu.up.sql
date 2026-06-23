-- ERP Item Master menu (Level 3 under Finance > Master — FINANCE_MASTER uuid 00000000-0000-0000-0002-000000000002).
-- Level-3 UUID 00000000-0000-0000-0003-000000000041 verified available.

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES (
    '00000000-0000-0000-0003-000000000041',
    '00000000-0000-0000-0002-000000000002',
    'FINANCE_ERP_ITEM',
    'ERP Item Master',
    '/finance/erp-items',
    'Link2',
    'finance',
    3,
    41,
    TRUE, TRUE, 'seed'
)
ON CONFLICT (menu_code) DO NOTHING;

-- Permissions for ERP Item management.
INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.erpitem.view',   'View ERP Item',   'View ERP item list and details', 'finance', 'master', 'view',   TRUE, 'seed'),
    ('finance.master.erpitem.create', 'Create ERP Item', 'Create new ERP items',           'finance', 'master', 'create', TRUE, 'seed'),
    ('finance.master.erpitem.update', 'Update ERP Item', 'Update existing ERP items',      'finance', 'master', 'update', TRUE, 'seed'),
    ('finance.master.erpitem.delete', 'Delete ERP Item', 'Delete ERP items',               'finance', 'master', 'delete', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- Assign all 4 permissions to the SUPER_ADMIN role.
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
         CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'finance.master.erpitem.%'
ON CONFLICT DO NOTHING;
