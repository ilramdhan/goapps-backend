-- H013: dashboard group WORKFORCE + dashboard R1/R2 (PRD §3) + jadwal job HR (PRD §8). Idempotent.
-- Chart 2 dimensi (stacked_bar dept × jenis, heatmap minggu × dept) memakai jalur pivot planner
-- (x_axis_field × stack_by_field / y_axis_field, hanya group_1/group_2/period; group_3 = NIK tidak
-- pernah jadi sumbu pivot, HD-07). Level karyawan hanya lewat drill (gerbang workforceemployee).
-- compare_modes kosong: jalur pivot tidak membuat overlay pembanding (Δ ada di KPI).
-- Semua dashboard memakai periode payroll 26–25 (HD-06) dan whitelist role DIREKSI_GM & HR (PRD §7).
BEGIN;

INSERT INTO bi_dashboard_group (group_code, group_name, icon, display_order, is_active, created_at)
VALUES ('WORKFORCE', 'Workforce', 'Users', 45, TRUE, NOW())
ON CONFLICT (group_code) DO NOTHING;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT v.code, v.title, v.descr, v.ftype, NULL, v.grain, v.def_period, 'PAYROLL_26_25',
       v.chart, v.cfg::jsonb, v.layout::jsonb, v.cmp::jsonb, v.kpi::jsonb,
       v.drill, v.max_drill, 1800, 0, v.ord, grp.group_id, TRUE
FROM grp, (VALUES
  -- R1 utama: stacked bar dept × jenis OT; drill lewat tabel: dept → jenis → karyawan (NIK).
  ('HR_OT_DEPT', 'Overtime per Departemen & Jenis',
   'R1: jam OT aktual (tier 1+2+3; Change Off = calc, HD-14) per dept × jenis, periode payroll 26–25',
   'HR_OT_HOURS', 'MONTHLY', 'THIS_MONTH', 'stacked_bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","stack_by_field":"group_2","number_format":"raw","decimals":1,"drill_to":"group_2","empty_message":"Belum ada data OT untuk periode ini"}',
   '{"secondary_charts":[{"title":"Dept × Jenis OT (jam) — klik dept untuk drill","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full"}]}',
   '[]',
   '[{"label":"Total Jam OT","value_field":"value","agg":"sum","compare":"MoM","format":"raw","decimals":1,"show_sparkline":true,"sparkline_periods":12},
     {"label":"% Jam OT oleh 10% Karyawan Teratas","agg":"cross_ratio","cross_ratio_numerator_type":"HR_OT_PARETO","numerator_group_1":"TOP_10PCT","cross_ratio_denominator_type":"HR_OT_PARETO","denominator_group_1":"TOTAL","scale":100,"compare":"none","format":"raw","decimals":1}]',
   TRUE, 2, 10),
  -- R1 sekunder: tren jam OT harian.
  ('HR_OT_TREND', 'Tren Jam OT Harian',
   'R1: total jam OT per hari dalam periode payroll',
   'HR_OT_HOURS', 'DAILY', 'THIS_MONTH', 'line',
   '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":1,"empty_message":"Belum ada data OT"}',
   NULL, '[]',
   '[{"label":"Total Jam OT","value_field":"value","agg":"sum","compare":"none","format":"raw","decimals":1}]',
   FALSE, 1, 20),
  -- R2 utama: heatmap minggu × dept (jumlah karyawan over-limit). Minggu d2 = minggu berjalan (parsial).
  ('HR_OT_OVERLIMIT', 'Kepatuhan Lembur 14 Jam/Minggu',
   'R2: karyawan > 14 jam/minggu (Senin–Minggu, K6); minggu terakhir = minggu berjalan (parsial)',
   'HR_OT_WEEK_STATUS', 'WEEKLY', 'THIS_MONTH', 'heatmap',
   '{"x_axis_field":"period","y_axis_field":"group_1","value_field":"value","filter_group_2":"OVER_LIMIT","number_format":"raw","decimals":0,"empty_message":"Tidak ada karyawan over-limit"}',
   '{"secondary_charts":[{"title":"Over-limit per Minggu × Dept (orang)","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":0},"span":"full","drill_enabled":false}]}',
   '[]',
   '[{"label":"Over-limit Minggu Lalu","value_field":"value","agg":"sum","period":"last_week","filter_group_2":"OVER_LIMIT","compare":"MoM","format":"raw","decimals":0},
     {"label":"Minggu Berjalan (parsial): Sudah Over","value_field":"value","agg":"sum","period":"this_week","filter_group_2":"OVER_LIMIT","compare":"none","format":"raw","decimals":0},
     {"label":"Minggu Berjalan (parsial): Mendekati Batas","value_field":"value","agg":"sum","period":"this_week","filter_group_2":"NEAR_LIMIT","compare":"none","format":"raw","decimals":0},
     {"label":"Kejadian Over-limit (orang-minggu)","value_field":"value","agg":"sum","filter_group_2":"OVER_LIMIT","compare":"none","format":"raw","decimals":0}]',
   FALSE, 1, 30),
  -- R2 sekunder: histogram orang-minggu per kelompok jam, ditumpuk per dept.
  ('HR_OT_WEEK_HIST', 'Distribusi Jam Lembur Mingguan',
   'R2: orang-minggu per kelompok jam (0–7 / 7–14 / 14–18 / >18)',
   'HR_OT_WEEK_BUCKET', 'WEEKLY', 'THIS_MONTH', 'stacked_bar',
   '{"x_axis_field":"group_2","y_axis_field":"value","stack_by_field":"group_1","number_format":"raw","decimals":0}',
   NULL, '[]', '[]', FALSE, 1, 40),
  -- R2 detail: dept → status minggu → karyawan (jam per minggu); NIK hanya dengan workforceemployee.view.
  ('HR_OT_WEEK_DETAIL', 'Jam Lembur Mingguan per Karyawan',
   'R2: jam hitung batas per dept → status (OVER/NEAR/UNDER) → karyawan',
   'HR_OT_WEEK', 'WEEKLY', 'LAST_WEEK', 'data_table',
   '{"x_axis_field":"group_1","y_axis_field":"value","number_format":"raw","decimals":2,"view_configs":{"data_table":{"title_template":"Jam Lembur Mingguan","drill_enabled":true,"hint":"Klik dept → status → karyawan"}}}',
   NULL, '[]', '[]', TRUE, 2, 50)
) AS v(code, title, descr, ftype, grain, def_period, chart, cfg, layout, cmp, kpi, drill, max_drill, ord)
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code IN ('HR_OT_DEPT', 'HR_OT_TREND', 'HR_OT_OVERLIMIT', 'HR_OT_WEEK_HIST', 'HR_OT_WEEK_DETAIL')
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

-- Jadwal WIB apa pun TZ server (container finance = UTC): 05:00 data H-1, run siang untuk peringatan
-- dini R2 (PRD §8). refresh_mvs: MV g1/g2 di-refresh setelah load supaya dashboard langsung terbaru.
UPDATE bi_job
SET schedule_cron = 'CRON_TZ=Asia/Jakarta 0 5 * * *',
    config = config || '{"refresh_mvs": true}'::jsonb
WHERE job_name = 'ETL_HR_OT_DAILY' AND schedule_cron = '0 5 * * *';

INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT 'ETL_HR_OT_MIDDAY', source_id, target_type, 'CRON_TZ=Asia/Jakarta 0 12 * * *', NULL,
       (config - 'near_limit_hours') || '{"refresh_mvs": true}'::jsonb, TRUE
FROM bi_job WHERE job_name = 'ETL_HR_OT_DAILY'
ON CONFLICT (job_name) DO NOTHING;

COMMIT;
