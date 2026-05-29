BEGIN;
UPDATE bi_dashboard
  SET
    kpi_config = '[
      {"label":"Current Month Net Profit","value_field":"display_value","agg":"sum","compare":"MoM","period":"current_month","format":"currency_thousands","show_sparkline":true,"sparkline_periods":12},
      {"label":"Avg Monthly Net Profit","value_field":"display_value","agg":"avg","compare":"none","period":"l12m","format":"currency_thousands","decimals":1},
      {"label":"YTD Net Profit","value_field":"display_value","agg":"sum","compare":"YTD_vs_LY","period":"ytd","format":"currency_thousands"},
      {"label":"L12M Total","value_field":"display_value","agg":"sum","compare":"none","period":"l12m","format":"currency_thousands"}
    ]'::jsonb,
    chart_config = chart_config - 'view_configs'
WHERE dashboard_code = 'NET_PROFIT';
COMMIT;
