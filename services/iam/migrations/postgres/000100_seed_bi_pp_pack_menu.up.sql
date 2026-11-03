-- P006: menu "Packed Production" di bawah Executive Dashboard (BI_PARENT), sejajar Workforce (PP-09).
-- UUID 0003-...060 dipilih agar tidak bentrok dengan menu Workforce HR (057-059). Idempotent.
BEGIN;

DO $$
DECLARE
    clash TEXT;
BEGIN
    SELECT m.menu_code INTO clash FROM mst_menu m
    WHERE m.menu_id = '00000000-0000-0000-0003-000000000060'::uuid AND m.menu_code <> 'BI_PP_PACK';
    IF clash IS NOT NULL THEN
        RAISE EXCEPTION 'P006: menu_id 0003-...060 sudah dipakai menu lain: %', clash;
    END IF;
END $$;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000060', '00000000-0000-0000-0002-000000000020',
     'BI_PP_PACK', 'Packed Production', '/finance/bi/PP_PACK_SUMMARY', 'Package',
     'finance', 3, 50, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
JOIN mst_permission p ON p.permission_code = 'finance.bi.dashboard.view' AND p.is_active = TRUE
WHERE m.menu_code = 'BI_PP_PACK'
ON CONFLICT (menu_id, permission_id) DO NOTHING;

COMMIT;
