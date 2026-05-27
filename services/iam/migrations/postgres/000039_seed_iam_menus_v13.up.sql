-- Menus for v1.3 admin pages.
-- Parents (existing): ADMIN_MASTER_DATA = 0002-000000000013 (administrator), FINANCE_MASTER = 0002-000000000002 (finance).
-- Next available level-3 sequence: 0003-00000000001c (after 000033 used up to ...001b).

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by) VALUES
    ('00000000-0000-0000-0003-00000000001c', '00000000-0000-0000-0002-000000000013', 'ADMIN_WORKFLOW_TEMPLATE', 'Workflow Templates', '/administrator/master/workflow-templates', 'GitBranch', 'iam',     3, 60, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-00000000001d', '00000000-0000-0000-0002-000000000002', 'FINANCE_PRODUCT_TYPE',    'Product Types',      '/finance/master/product-types',            'Tags',      'finance', 3, 25, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- Menu permissions — gate by .view permission
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-00000000001c', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'iam.master.workflowtemplate.view' AND is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-00000000001d', permission_id, 'seed'
FROM mst_permission WHERE permission_code = 'finance.master.producttype.view' AND is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;
