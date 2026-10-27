-- H023 (HD-18): HR_OT_EMPLOYEES menjadi tabel "Employee Overtime" (table_mode=employee_ot): semua
-- karyawan ber-OT di periode (R1) atau minggu R2, dikelompokkan per divisi/dept saat query.
BEGIN;

UPDATE bi_dashboard
SET dashboard_title = 'Employee Overtime',
    description     = 'Jam OT per karyawan di periode payroll (R1) dan jam hitung batas minggu terakhir periode (R2: kelebihan di atas 14 jam, minggu berturut-turut); kelompokkan per divisi/dept; klik NIK untuk jam per hari',
    chart_config    = '{"table_mode":"employee_ot","x_axis_field":"group_1","y_axis_field":"value","number_format":"raw","decimals":2,"empty_message":"Tidak ada karyawan dengan OT di periode ini","view_configs":{"data_table":{"title_template":"Employee Overtime","drill_enabled":true,"hint":"Klik NIK untuk jam per hari"}}}'::jsonb,
    updated_at      = NOW()
WHERE dashboard_code = 'HR_OT_EMPLOYEES';

COMMIT;
