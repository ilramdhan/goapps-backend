-- Update EBITDA dashboard: 4 KPI cards matching the HTML UX reference,
-- plus view_configs for dynamic title/drill/hint per chart type.
BEGIN;

UPDATE bi_dashboard
  SET
    kpi_config = '[
      {
        "label": "Current Month EBITDA",
        "value_field": "display_value",
        "agg": "sum",
        "compare": "MoM",
        "period": "current_month",
        "format": "currency_thousands",
        "show_sparkline": true,
        "sparkline_periods": 12
      },
      {
        "label": "YTD EBITDA",
        "value_field": "display_value",
        "agg": "sum",
        "compare": "YTD_vs_LY",
        "period": "ytd",
        "format": "currency_thousands"
      },
      {
        "label": "Avg Monthly (L12M)",
        "value_field": "display_value",
        "agg": "avg",
        "compare": "none",
        "period": "l12m",
        "format": "currency_thousands",
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
        "waterfall": {
          "title_template": "EBITDA Breakdown — {period}",
          "drill_enabled": true,
          "hint": "Click any bar to drill into detail components"
        },
        "bar": {
          "title_template": "EBITDA by Component — {period}",
          "drill_enabled": true,
          "hint": "Component-level bar chart — click to drill"
        },
        "line": {
          "title_template": "EBITDA Trend Over Time",
          "drill_enabled": false,
          "hint": "Monthly EBITDA values — select a period below to inspect breakdown"
        },
        "data_table": {
          "title_template": "EBITDA Data — {period}",
          "drill_enabled": true,
          "hint": "Click any row to drill into components"
        }
      }
    }'::jsonb
WHERE dashboard_code = 'EBITDA';

COMMIT;
