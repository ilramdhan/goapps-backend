-- H018: tabel karyawan R2 (kronis/kelebihan, minggu terakhir) + drill hari. table_mode=employee_week
-- membaca bi_fact_metric langsung dan digerbangi workforceemployee.view (menampilkan NIK, HD-07). Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT 'HR_OT_EMPLOYEES', 'Karyawan Over-limit Mingguan',
       'R2: karyawan di atas 14 jam pada minggu terakhir periode, kelebihan dan minggu berturut-turut; klik NIK untuk jam per hari',
       'HR_OT_WEEK', NULL, 'WEEKLY', 'THIS_MONTH', 'PAYROLL_26_25',
       'data_table',
       '{"table_mode":"employee_week","x_axis_field":"group_1","y_axis_field":"value","number_format":"raw","decimals":2,"empty_message":"Tidak ada karyawan over-limit","view_configs":{"data_table":{"title_template":"Karyawan Over-limit","drill_enabled":true,"hint":"Klik NIK untuk jam per hari"}}}'::jsonb,
       NULL, '[]'::jsonb, '[]'::jsonb,
       TRUE, 1, 1800, 0, 60, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code = 'HR_OT_EMPLOYEES'
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

COMMIT;
