-- H013 / H019 / H024 / H034 / H060 (squash 000097–000100 branch HR; belum pernah jalan di prod): menu Workforce
-- di bawah Executive Dashboard (BI_PARENT 00000000-0000-0000-0002-000000000020, migrasi 000044).
-- Bentuk akhir: satu menu tampil "Workforce" (BI_HR_OT_DEPT, pemegang finance.bi.workforce.view). Dua halaman
-- lain dijangkau lewat tab di viewer; menunya tersembunyi (is_visible=FALSE) tetapi tetap ada untuk akses URL
-- langsung dan pemetaan permission: Kepatuhan 14 Jam (workforce.view) dan Employee Overtime (data NIK,
-- workforceemployee.view; HD-07, HD-10).
-- UUID level-3 057–059 (053–056 dipakai ERP/Superba main). Kalau menu_id sudah dipakai menu lain, migrasi
-- gagal (ON CONFLICT (menu_code) tidak melindungi PK). Idempotent.
BEGIN;

DO $$
DECLARE
    clash TEXT;
BEGIN
    SELECT string_agg(m.menu_id || ' = ' || m.menu_code, ', ') INTO clash
    FROM mst_menu m
    JOIN (VALUES
        ('00000000-0000-0000-0003-000000000057'::uuid, 'BI_HR_OT_DEPT'),
        ('00000000-0000-0000-0003-000000000058'::uuid, 'BI_HR_OT_OVERLIMIT'),
        ('00000000-0000-0000-0003-000000000059'::uuid, 'BI_HR_OT_EMPLOYEES')
    ) AS v(menu_id, menu_code) ON v.menu_id = m.menu_id AND v.menu_code <> m.menu_code;
    IF clash IS NOT NULL THEN
        RAISE EXCEPTION 'H013: menu_id Workforce sudah dipakai menu lain: %', clash;
    END IF;
END $$;

INSERT INTO mst_menu (
    menu_id, parent_id, menu_code, menu_title, menu_url, icon_name,
    service_name, menu_level, sort_order, is_visible, is_active, created_by
) VALUES
    ('00000000-0000-0000-0003-000000000057', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_DEPT', 'Workforce', '/finance/bi/HR_OT_DEPT', 'Users',
     'finance', 3, 40, TRUE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000058', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_OVERLIMIT', 'Kepatuhan Lembur 14 Jam', '/finance/bi/HR_OT_OVERLIMIT', 'AlertTriangle',
     'finance', 3, 41, FALSE, TRUE, 'seed'),
    ('00000000-0000-0000-0003-000000000059', '00000000-0000-0000-0002-000000000020',
     'BI_HR_OT_EMPLOYEES', 'Employee Overtime', '/finance/bi/HR_OT_EMPLOYEES', 'Users',
     'finance', 3, 42, FALSE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
JOIN (VALUES
    ('BI_HR_OT_DEPT',      'finance.bi.workforce.view'),
    ('BI_HR_OT_OVERLIMIT', 'finance.bi.workforce.view'),
    ('BI_HR_OT_EMPLOYEES', 'finance.bi.workforceemployee.view')
) AS v(menu_code, permission_code) ON v.menu_code = m.menu_code
JOIN mst_permission p ON p.permission_code = v.permission_code AND p.is_active = TRUE
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Backfill mst_permission.menu_id (konvensi 000066, 000095 bagian 4).
UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM (VALUES
    ('finance.bi.workforce.view',         'BI_HR_OT_DEPT'),
    ('finance.bi.workforceemployee.view', 'BI_HR_OT_EMPLOYEES')
) AS v(permission_code, menu_code)
JOIN mst_menu m ON m.menu_code = v.menu_code
WHERE p.permission_code = v.permission_code
  AND p.menu_id IS NULL;

COMMIT;
