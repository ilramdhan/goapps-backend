-- P014 / PP-15: 000603 memasang dashboard PP_PACK_* di grup OPERATIONS, padahal production hanya punya grup
-- PRODUCTION (OPERATIONS sudah tidak ada) sehingga INSERT ... SELECT FROM grp menghasilkan 0 baris tanpa error.
-- Migrasi ini memasang 4 dashboard yang sama persis dengan 000603 di grup PRODUCTION (fallback OPERATIONS),
-- lalu gagal keras bila grup tidak ada atau hasil akhirnya bukan 4 dashboard. Lingkungan yang sudah benar dari
-- 000603 tidak diubah (ON CONFLICT DO NOTHING). Baris buatan migrasi ini ditandai layout_config.seeded_by='000606'
-- supaya down hanya menghapus yang dibuatnya. 000603 tidak diubah (sudah jalan di production).
BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM bi_dashboard_group WHERE group_code IN ('PRODUCTION', 'OPERATIONS')) THEN
        RAISE EXCEPTION 'P014: grup bi_dashboard_group PRODUCTION maupun OPERATIONS tidak ada';
    END IF;
END $$;

WITH grp AS (
    SELECT group_id FROM bi_dashboard_group
     WHERE group_code IN ('PRODUCTION', 'OPERATIONS')
     ORDER BY CASE group_code WHEN 'PRODUCTION' THEN 1 ELSE 2 END
     LIMIT 1
)
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
   '{"nav_tab":{"group":"PACKPROD","label":"Summary","permission":"finance.bi.dashboard.view"},"seeded_by":"000606"}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0},
     {"label":"AX % (bulan ini)","agg":"cross_ratio","numerator_metric":"PACK_AX","denominator_metric":"PACK_TOTAL","period":"current_month","compare":"none","format":"percent","decimals":1},
     {"label":"FQ % (bulan ini)","agg":"cross_ratio","numerator_metric":"PACK_FQ","denominator_metric":"PACK_TOTAL","period":"current_month","compare":"none","format":"percent","decimals":1}]',
   TRUE, 2, 10),
  ('PP_PACK_PRODUCT', 'Packed Production per Produk',
   'Kgs packed per produk (merge_no; tanpa Boughtout/Rewinding)',
   'PP_PACK_PRODUCT', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data produk untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Produk","permission":"finance.bi.dashboard.view"},"seeded_by":"000606"}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 20),
  ('PP_PACK_MACHINE', 'Packed Production per Mesin',
   'Kgs packed per mesin (mc_no, urut mach_order_rep; hanya grade_eligible=Y)',
   'PP_PACK_MACHINE', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data mesin untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Mesin","permission":"finance.bi.dashboard.view"},"seeded_by":"000606"}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 30),
  ('PP_PACK_POLY', 'Packed Production per Poly',
   'Kgs packed per jenis poly (poly_type)',
   'PP_PACK_POLY', 'bar',
   '{"x_axis_field":"group_1","y_axis_field":"value","category_field":"dimension_key","metric_filter":{"include_metrics":["PACK_TOTAL"]},"number_format":"raw","decimals":0,"empty_message":"Belum ada data poly untuk periode ini"}',
   '{"nav_tab":{"group":"PACKPROD","label":"Poly","permission":"finance.bi.dashboard.view"},"seeded_by":"000606"}',
   '[{"label":"Total Kgs (bulan ini)","value_field":"value","metric_name":"PACK_TOTAL","agg":"sum","period":"current_month","compare":"MoM","format":"raw","decimals":0}]',
   FALSE, 1, 40)
) AS v(code, title, descr, ftype, chart, cfg, layout, kpi, drill, max_drill, ord)
ON CONFLICT (dashboard_code) DO NOTHING;

DO $$
BEGIN
    IF (SELECT count(*) FROM bi_dashboard
         WHERE dashboard_code IN ('PP_PACK_SUMMARY', 'PP_PACK_PRODUCT', 'PP_PACK_MACHINE', 'PP_PACK_POLY')
           AND group_id IS NOT NULL AND deleted_at IS NULL AND is_active) <> 4 THEN
        RAISE EXCEPTION 'P014: dashboard PP_PACK_* aktif bukan 4 setelah seed (cek soft-delete / is_active)';
    END IF;
END $$;

COMMIT;
