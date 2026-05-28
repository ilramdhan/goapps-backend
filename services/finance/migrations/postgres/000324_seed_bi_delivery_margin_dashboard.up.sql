-- Seed: Delivery Margin dashboard config.
-- chart_type='line' with metric_filter (multi-metric: GROSS_SALES, NETT_SALES, COST_PROD, MARGIN).
-- filter_chips drives the Delivery Type + Category chip filters in the viewer.
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
    'DELIVERY_MARGIN',
    'Delivery Margin',
    'Sales margin by delivery type and product category (multi-metric)',
    'SALES', '', 'MONTHLY', 'L12M',
    'line',
    '{
      "x_axis_field": "period",
      "y_axis_field": "display_value",
      "number_format": "currency_thousands",
      "decimals": 1,
      "metric_filter": {
        "include_metrics": ["GROSS_SALES","NETT_SALES","COST_PROD","MARGIN"]
      },
      "series_colors": {
        "GROSS_SALES": "#1F4E79",
        "NETT_SALES":  "#2E75B6",
        "COST_PROD":   "#a32d2d",
        "MARGIN":      "#1d9e75"
      },
      "filter_chips": ["group_1","group_2"],
      "empty_message": "No delivery margin data for selected period"
    }'::jsonb,
    '{
      "secondary_charts": [
        {
          "title": "Monthly Detail",
          "chart_type": "data_table",
          "span": "full"
        }
      ]
    }'::jsonb,
    '["MoM","YoY","YTD","R12"]'::jsonb,
    '[
      {"label":"Current Month Margin","value_field":"display_value","agg":"sum","compare":"MoM","period":"current_month","format":"currency_thousands","show_sparkline":true,"sparkline_periods":12},
      {"label":"YTD Margin","value_field":"display_value","agg":"sum","compare":"YTD_vs_LY","period":"ytd","format":"currency_thousands"},
      {"label":"L12M Total Margin","value_field":"display_value","agg":"sum","compare":"none","period":"l12m","format":"currency_thousands"}
    ]'::jsonb,
    TRUE, 3, 1800, 0, 30, grp.group_id, TRUE
FROM grp
ON CONFLICT (dashboard_code) DO NOTHING;

COMMIT;
