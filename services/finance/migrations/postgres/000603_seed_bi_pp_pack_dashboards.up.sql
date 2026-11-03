-- P006: dashboard Packed Production (PP_PACK_*), 4 tab (PP-09) dalam grup OPERATIONS.
-- Summary memakai jalur MV (single_metric PACK_TOTAL, drill dept -> jenis transaksi -> grup);
-- Product/Machine/Poly memakai category_field=dimension_key (PP-10, label/urutan dari staging, PP-04).
-- KPI AX%/FQ% = cross_ratio antar metric (PACK_AX/PACK_TOTAL), format percent. Akses: siapa pun dengan
-- finance.bi.dashboard.view (tanpa whitelist bi_dashboard_role). Idempotent.
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'OPERATIONS')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT v.code, v.title, v.descr, v.ftype, NULL, 'MONTHLY', 'THIS_MONTH',
       v.chart, v.cfg::jsonb, v.layout::jsonb, '[]'::jsonb, v.kpi::jsonb,
       v.drill, v.max_drill, 1800, 0, v.ord, grp.group_id, TRUE
FROM grp, (VALUES
  ('PP_PACK_SUMMARY', 'Packed Production',
   'Kgs packed per departemen / jenis transaksi / grup (pengganti VIEW_PACKDET_MAIL_ALERT_SUM)',
   'PP_PACK_SUMMARY', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","single_metric":"PACK_TOTAL","number_format":"raw","decimals":0,"drill_to":"group_2","empty_message":"Belum ada data packed production untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Summary","permission":"finance.bi.dashboard.view"}}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0},
     {"label":"AX % (bulan ini)","agg":"cross_ratio","numerator_metric":"PACK_AX","denominator_metric":"PACK_TOTAL","period":"current_month","compare":"none","format":"percent","decimals":1},
     {"label":"FQ % (bulan ini)","agg":"cross_ratio","numerator_metric":"PACK_FQ","denominator_metric":"PACK_TOTAL","period":"current_month","compare":"none","format":"percent","decimals":1}]',
   TRUE, 2, 10),
  ('PP_PACK_PRODUCT', 'Packed Production per Produk',
   'Kgs packed per produk (merge_no; tanpa Boughtout/Rewinding)',
   'PP_PACK_PRODUCT', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data produk untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Produk","permission":"finance.bi.dashboard.view"}}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 20),
  ('PP_PACK_MACHINE', 'Packed Production per Mesin',
   'Kgs packed per mesin (mc_no, urut mach_order_rep; hanya grade_eligible=Y)',
   'PP_PACK_MACHINE', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data mesin untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Mesin","permission":"finance.bi.dashboard.view"}}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 30),
  ('PP_PACK_POLY', 'Packed Production per Poly',
   'Kgs packed per jenis poly (poly_type)',
   'PP_PACK_POLY', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data poly untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Poly","permission":"finance.bi.dashboard.view"}}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 40)
) AS v(code, title, descr, ftype, chart, cfg, layout, kpi, drill, max_drill, ord)
ON CONFLICT (dashboard_code) DO NOTHING;

-- Summary membaca mv_bi_metric_g1/g2; MV hanya di-refresh job MV_REFRESH (00:05, 04:05, ...), sehingga
-- tanpa job ini Summary tertinggal dari tab lain sampai refresh berikutnya. ETL_PP_PACK_DAILY 07:00
-- selesai dalam hitungan detik (P005), jadi refresh 07:15 cukup.
WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT 'MV_REFRESH_PP_PACK', src.source_id, 'ALL', '15 7 * * *', NULL,
       '{"kind": "mv_refresh"}'::jsonb, TRUE
FROM src
ON CONFLICT (job_name) DO NOTHING;

COMMIT;
