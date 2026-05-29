BEGIN;

UPDATE bi_dashboard
  SET
    kpi_config = '[{"label":"Current Month Margin","value_field":"display_value","agg":"sum","compare":"MoM","period":"current_month","format":"currency_thousands","show_sparkline":true,"sparkline_periods":12},{"label":"YTD Margin","value_field":"display_value","agg":"sum","compare":"YTD_vs_LY","period":"ytd","format":"currency_thousands"},{"label":"L12M Total Margin","value_field":"display_value","agg":"sum","compare":"none","period":"l12m","format":"currency_thousands"}]'::jsonb,
    chart_config = chart_config - 'view_configs'
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
