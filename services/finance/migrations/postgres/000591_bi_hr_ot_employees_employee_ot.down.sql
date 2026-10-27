-- Kembalikan HR_OT_EMPLOYEES ke tabel karyawan over-limit R2 (000589, table_mode=employee_week).
BEGIN;

UPDATE bi_dashboard
SET dashboard_title = 'Karyawan Over-limit Mingguan',
    description     = 'R2: karyawan di atas 14 jam pada minggu terakhir periode, kelebihan dan minggu berturut-turut; klik NIK untuk jam per hari',
    chart_config    = '{"table_mode":"employee_week","x_axis_field":"group_1","y_axis_field":"value","number_format":"raw","decimals":2,"empty_message":"Tidak ada karyawan over-limit","view_configs":{"data_table":{"title_template":"Karyawan Over-limit","drill_enabled":true,"hint":"Klik NIK untuk jam per hari"}}}'::jsonb,
    updated_at      = NOW()
WHERE dashboard_code = 'HR_OT_EMPLOYEES';

COMMIT;
