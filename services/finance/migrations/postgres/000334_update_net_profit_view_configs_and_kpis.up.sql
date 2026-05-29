-- Update NET_PROFIT dashboard: 4 KPI cards (including cross_ratio for NP Margin vs EBITDA),
-- view_configs for dynamic title/drill/hint per chart type.
-- layout_config updated to keep dual_line + monthly detail.
BEGIN;

UPDATE bi_dashboard
  SET
    kpi_config = '[
      {
        "label": "Current Month Net Profit",
        "value_field": "display_value",
        "agg": "sum",
        "compare": "MoM",
        "period": "current_month",
        "format": "currency_thousands",
        "show_sparkline": true,
        "sparkline_periods": 12
      },
      {
        "label": "YTD Net Profit",
        "value_field": "display_value",
        "agg": "sum",
        "compare": "YTD_vs_LY",
        "period": "ytd",
        "format": "currency_thousands"
      },
      {
        "label": "Net Profit Margin",
        "agg": "cross_ratio",
        "numerator_group_1": "NET PROFIT",
        "denominator_group_1": "EBITDA",
        "scale": 100,
        "period": "l12m",
        "compare": "none",
        "format": "percent",
        "decimals": 1
      },
      {
        "label": "L12M Total",
        "value_field": "display_value",
        "agg": "sum",
        "compare": "none",
        "period": "l12m",
        "format": "currency_thousands"
      }
    ]'::jsonb,
    chart_config = chart_config || '{
      "view_configs": {
        "line": {
          "title_template": "Net Profit Trend",
          "drill_enabled": false,
          "hint": "Monthly net profit — select comparison mode above"
        },
        "bar": {
          "title_template": "Net Profit by Month",
          "drill_enabled": false,
          "hint": "Bar view"
        },
        "area": {
          "title_template": "Net Profit Area Trend",
          "drill_enabled": false,
          "hint": ""
        },
        "data_table": {
          "title_template": "Net Profit Monthly Detail",
          "drill_enabled": false,
          "hint": ""
        }
      }
    }'::jsonb
WHERE dashboard_code = 'NET_PROFIT';

COMMIT;
