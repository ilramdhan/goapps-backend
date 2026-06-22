-- Migration 000061: Add Import / Export Jobs menu under FINANCE_PRODUCT_COSTING
--
-- Menu UUID convention (deterministic per level):
--   Level-3 next available: 00000000-0000-0000-0003-000000000040
--
-- Permission policy: NO entries in menu_permissions → visible to ALL authenticated users.
-- Any user who can perform imports/exports must be able to monitor job status.

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url,
    icon_name, service_name, menu_level, sort_order,
    is_visible, is_active, created_by
) VALUES (
    '00000000-0000-0000-0003-000000000040',
    '00000000-0000-0000-0002-000000000015',  -- FINANCE_PRODUCT_COSTING
    'FINANCE_IMPORT_JOBS',
    'Import / Export Jobs',
    '/finance/import-jobs',
    'ClipboardList',
    'finance',
    3,
    99,
    true,
    true,
    'seed'
) ON CONFLICT (menu_code) DO NOTHING;
