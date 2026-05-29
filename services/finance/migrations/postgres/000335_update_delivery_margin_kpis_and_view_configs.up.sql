-- Update DELIVERY_MARGIN dashboard: 4 KPI cards using metric_name for SALES multi-metric data,
-- plus view_configs for dynamic title/hint per chart type.
BEGIN;

UPDATE bi_dashboard
  SET
    kpi_config = '[
      {
        "label": "Current Month Margin",
        "value_field": "display_value",
        "metric_name": "MARGIN",
        "agg": "sum",
        "compare": "MoM",
        "period": "current_month",
        "format": "currency_thousands",
        "show_sparkline": true,
        "sparkline_periods": 12
      },
      {
        "label": "YTD Margin",
        "value_field": "display_value",
        "metric_name": "MARGIN",
        "agg": "sum",
        "compare": "YTD_vs_LY",
        "period": "ytd",
        "format": "currency_thousands"
      },
      {
        "label": "Avg Monthly Margin",
        "value_field": "display_value",
        "metric_name": "MARGIN",
        "agg": "avg",
        "compare": "none",
        "period": "l12m",
        "format": "currency_thousands",
        "decimals": 1
      },
      {
        "label": "L12M Total Margin",
        "value_field": "display_value",
        "metric_name": "MARGIN",
        "agg": "sum",
        "compare": "none",
        "period": "l12m",
        "format": "currency_thousands"
      }
    ]'::jsonb,
    chart_config = chart_config || '{
      "view_configs": {
        "line": {
          "title_template": "Delivery Margin Trend",
          "drill_enabled": false,
          "hint": "Multi-metric trend — toggle series in legend"
        },
        "bar": {
          "title_template": "Delivery Margin by Period",
          "drill_enabled": false,
          "hint": "Bar view"
        },
        "area": {
          "title_template": "Delivery Margin Area",
          "drill_enabled": false,
          "hint": ""
        },
        "data_table": {
          "title_template": "Delivery Margin Monthly Detail",
          "drill_enabled": false,
          "hint": ""
        }
      }
    }'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
