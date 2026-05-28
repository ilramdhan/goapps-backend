-- Seed: reference dashboards EBITDA (waterfall) + NET_PROFIT (mixed bar+line).
BEGIN;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'FINANCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT
    'EBITDA',
    'EBITDA Performance',
    'Earnings Before Interest, Tax, Depreciation & Amortization',
    'MIS', 'EBITDA', 'MONTHLY', 'L12M',
    'waterfall',
    '{"x_axis_field":"group_2","y_axis_field":"display_value","positive_color":"#1d9e75","negative_color":"#a32d2d","total_color":"#534AB7","number_format":"currency_thousands","decimals":1,"show_total_bar":true,"show_data_labels":true,"drill_to":"group_3","empty_message":"No data for selected period"}'::jsonb,
    NULL,
    '["MoM","QoQ","YoY","YTD","R12"]'::jsonb,
    '[{"label":"Current Month EBITDA","value_field":"display_value","agg":"sum","compare":"MoM","period":"current_month","format":"currency_thousands","show_sparkline":true,"sparkline_periods":12},
      {"label":"YTD EBITDA","value_field":"display_value","agg":"sum","compare":"YTD_vs_LY","period":"ytd","format":"currency_thousands"},
      {"label":"L12M Total","value_field":"display_value","agg":"sum","compare":"none","period":"l12m","format":"currency_thousands"}]'::jsonb,
    TRUE, 3, 1800, 0, 10, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

WITH grp AS (SELECT group_id FROM bi_dashboard_group WHERE group_code = 'FINANCE')
INSERT INTO bi_dashboard (
    dashboard_code, dashboard_title, description,
    filter_type, filter_group_1, periode_grain, default_period,
    chart_type, chart_config, layout_config, compare_modes, kpi_config,
    drill_enabled, max_drill_level, cache_ttl_sec, refresh_interval_sec,
    display_order, group_id, is_active
)
SELECT
    'NET_PROFIT',
    'Net Profit Trend',
    'Bottom-line profitability over time',
    'MIS', 'NET PROFIT', 'MONTHLY', 'L12M',
    'mixed',
    '{"x_axis_field":"period","y_axis_field":"display_value","primary_color":"#1d9e75","number_format":"currency_thousands","decimals":1,"show_data_labels":true,"series_defs":[{"name":"Net Profit","type":"bar","field":"display_value"},{"name":"YoY","type":"line","field":"yoy_value"}],"empty_message":"No data"}'::jsonb,
    '{"secondary_charts":[{"title":"Net Profit Trend (Line View)","chart_type":"line","chart_config":{"x_axis_field":"period","y_axis_field":"display_value","number_format":"currency_thousands","decimals":1},"span":"half"},{"title":"Monthly Detail","chart_type":"data_table","span":"half"}]}'::jsonb,
    '["MoM","YoY","YTD","R12"]'::jsonb,
    '[{"label":"Current Month Net Profit","value_field":"display_value","agg":"sum","compare":"MoM","period":"current_month","format":"currency_thousands","show_sparkline":true,"sparkline_periods":12},
      {"label":"Avg Monthly Net Profit","value_field":"display_value","agg":"avg","compare":"none","period":"l12m","format":"currency_thousands","decimals":1},
      {"label":"YTD Net Profit","value_field":"display_value","agg":"sum","compare":"YTD_vs_LY","period":"ytd","format":"currency_thousands"},
      {"label":"L12M Total","value_field":"display_value","agg":"sum","compare":"none","period":"l12m","format":"currency_thousands"}]'::jsonb,
    TRUE, 3, 1800, 0, 20, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

COMMIT;
