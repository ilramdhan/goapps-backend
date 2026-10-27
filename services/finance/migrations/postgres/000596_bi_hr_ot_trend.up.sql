-- H035–H041 / H060 (squash 000596–000601, 000603–000605, 000607 branch HR; belum pernah jalan di prod):
-- bentuk akhir dashboard "Tren OT" R1 + dashboard pendamping HR_OT_PARETO.
--
-- HR_OT_TREND (000588: tren harian) menjadi batang bertumpuk bulan × tipe OT (L12M, payroll 26–25).
-- Fakta MONTHLY HR_OT_HOURS (group_1 = dept, group_2 = tipe OT), HR_OT_EMP_COUNT, HR_HEADCOUNT (HD-06, HD-08).
-- Maks. 6 kartu KPI: 4 tipe OT + jam OT/karyawan + % karyawan lembur, period current_month + MoM.
-- Drill mati: sumbu x = periode, jadi baris tabel adalah bulan, bukan dept. Rincian dept lewat chip.
-- Chip dept: viewer menampilkan chip bila chart_config.filter_chips_group1 terisi; daftar sebenarnya
-- diganti nilai distinct HR_OT_HOURS saat dibuka, jadi isi di sini hanya pemicu + cadangan.
-- Kartu sekunder (dirender frontend dari /computed, tanpa ETL baru):
--   ot_mix (komposisi % per tipe), ot_compare (bulan terpilih vs MoM/YoY), tabel bulan × tipe,
--   ot_pareto (% jam OT oleh 10% karyawan teratas, sumber HR_OT_PARETO; rasio di frontend, HD-08/HD-09).
-- HR_OT_PARETO: dashboard tanpa tab, hanya sumber data kartu ot_pareto. chart_type 'line', bukan
-- stacked_bar, karena jalur pivot membaca mv_bi_metric_g2 yang membuang baris group_2 NULL.
-- Dashboard tier (HR_OT_TIER_MONTH) dan kualitas data (HR_OT_DQ_MONTH) sudah dicabut (HD-21) sehingga
-- tidak dibuat. Fakta HR_OT_DQ tetap dihitung ETL untuk rekon R1. Idempotent.
BEGIN;

UPDATE bi_dashboard
SET dashboard_title = 'Tren OT Bulanan per Tipe',
    description = 'R1: naik/turun jam OT antar bulan per tipe (Normal/Off/Holiday/Short Holiday/Change Off), periode payroll 26–25',
    filter_type = 'HR_OT_HOURS',
    periode_grain = 'MONTHLY',
    default_period = 'L12M',
    chart_type = 'stacked_bar',
    chart_config = '{"x_axis_field":"period","y_axis_field":"value","stack_by_field":"group_2","number_format":"raw","decimals":1,"empty_message":"Belum ada data OT"}'::jsonb
      || jsonb_build_object('filter_chips_group1', COALESCE(
           (SELECT jsonb_agg(g ORDER BY g) FROM (SELECT DISTINCT group_1 AS g FROM bi_fact_metric
             WHERE type = 'HR_OT_HOURS' AND group_1 IS NOT NULL) x),
           '["Finance"]'::jsonb)),
    layout_config = '{"secondary_charts":[
      {"title":"Komposisi Tipe OT (%)","chart_type":"ot_mix","chart_config":{"decimals":1},"span":"full","drill_enabled":false},
      {"title":"Perbandingan Periode","chart_type":"ot_compare","chart_config":{"metric":"VALUE","period_calendar":"PAYROLL_26_25","decimals":1},"span":"full","drill_enabled":false},
      {"title":"Jam OT per Bulan × Tipe OT (ikut filter dept)","chart_type":"data_table","chart_config":{"number_format":"raw","decimals":1},"span":"full","drill_enabled":false},
      {"title":"Konsentrasi Lembur: % Jam OT oleh 10% Karyawan Teratas","chart_type":"ot_pareto","chart_config":{"source_dashboard_code":"HR_OT_PARETO","top_key":"TOP_10PCT","total_key":"TOTAL","period_calendar":"PAYROLL_26_25","decimals":1},"span":"full","drill_enabled":false}
    ]}'::jsonb,
    compare_modes = '[]'::jsonb,
    kpi_config = '[
      {"label":"Normal","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Normal","compare":"MoM","format":"raw","decimals":1},
      {"label":"Off","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Off","compare":"MoM","format":"raw","decimals":1},
      {"label":"Holiday","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Holiday","compare":"MoM","format":"raw","decimals":1},
      {"label":"Short Holiday","value_field":"value","agg":"sum","period":"current_month","filter_group_2":"Short Holiday","compare":"MoM","format":"raw","decimals":1},
      {"label":"Jam OT per Karyawan","agg":"cross_ratio","period":"current_month","cross_ratio_numerator_type":"HR_OT_HOURS","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":1,"compare":"MoM","format":"raw","decimals":2},
      {"label":"% Karyawan Lembur","agg":"cross_ratio","period":"current_month","cross_ratio_numerator_type":"HR_OT_EMP_COUNT","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":100,"compare":"MoM","format":"raw","decimals":1}
    ]'::jsonb,
    drill_enabled = FALSE,
    max_drill_level = 1,
    display_order = 15
WHERE dashboard_code = 'HR_OT_TREND';

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'WORKFORCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period, period_calendar,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT
    'HR_OT_PARETO', 'Konsentrasi Lembur (Pareto)',
    'Sumber data kartu ot_pareto di HR_OT_TREND: jam OT 10% karyawan teratas vs total per bulan',
    'HR_OT_PARETO', NULL, 'MONTHLY', 'L12M', 'PAYROLL_26_25',
    'line',
    '{"x_axis_field":"period","y_axis_field":"value","number_format":"raw","decimals":1,"empty_message":"Belum ada data Pareto OT"}'::jsonb,
    NULL, '[]'::jsonb, '[]'::jsonb,
    FALSE, 1, 1800, 0, 16, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

INSERT INTO bi_dashboard_role (dashboard_id, role_code)
SELECT d.dashboard_id, r.role_code
FROM bi_dashboard d
CROSS JOIN (VALUES ('DIREKSI_GM'), ('HR')) AS r(role_code)
WHERE d.dashboard_code = 'HR_OT_PARETO'
ON CONFLICT (dashboard_id, role_code) DO NOTHING;

COMMIT;
