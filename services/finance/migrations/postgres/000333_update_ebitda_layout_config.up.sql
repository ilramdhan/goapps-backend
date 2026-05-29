-- Add layout_config to EBITDA with 2 secondary charts:
-- 1. EBITDA Trend (Last 12 Months) — line chart showing trend
-- 2. Component Detail — data_table filtered to selectedPeriod, rows clickable
BEGIN;

UPDATE bi_dashboard
  SET layout_config = '{
    "secondary_charts": [
      {
        "title": "EBITDA Trend (Last 12 Months)",
        "chart_type": "line",
        "available_chart_types": ["bar", "area"],
        "chart_config": {
          "x_axis_field": "period",
          "y_axis_field": "display_value",
          "number_format": "currency_thousands",
          "decimals": 1,
          "smooth": true
        },
        "span": "half"
      },
      {
        "title": "Component Detail",
        "chart_type": "data_table",
        "span": "half"
      }
    ]
  }'::jsonb
WHERE dashboard_code = 'EBITDA';

COMMIT;
