-- H060 (DOWN): buang HR_OT_PARETO dan kembalikan HR_OT_TREND ke tren jam OT harian (000588).
-- Fakta HR_OT_PARETO tidak dihapus karena dipakai KPI HR_OT_DEPT (000588).
BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_PARETO');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_PARETO';
UPDATE bi_dashboard
SET dashboard_title = 'Tren Jam OT Harian',
    description = 'R1: total jam OT per hari dalam periode payroll',
    filter_type = 'HR_OT_HOURS',
    periode_grain = 'DAILY', default_period = 'THIS_MONTH', chart_type = 'line',
    chart_config = '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":1,"empty_message":"Belum ada data OT"}'::jsonb,
    layout_config = NULL,
    compare_modes = '[]'::jsonb,
    kpi_config = '[{"label":"Total Jam OT","value_field":"value","agg":"sum","compare":"none","format":"raw","decimals":1}]'::jsonb,
    drill_enabled = FALSE, max_drill_level = 1, display_order = 20
WHERE dashboard_code = 'HR_OT_TREND';
COMMIT;
