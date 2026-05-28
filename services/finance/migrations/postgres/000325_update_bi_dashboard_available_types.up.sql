-- Add available_chart_types to existing dashboards so the viewer can offer a type switcher.
-- EBITDA waterfall → can also view as bar, line, data_table.
-- NET_PROFIT line → can also view as bar, area, data_table.
-- DELIVERY_MARGIN line (multi-metric) → can also view as bar, area, data_table.
BEGIN;

UPDATE bi_dashboard
  SET chart_config = chart_config || '{"available_chart_types":["bar","line","data_table"]}'::jsonb
WHERE dashboard_code = 'EBITDA';

UPDATE bi_dashboard
  SET chart_config = chart_config || '{"available_chart_types":["bar","area","data_table"]}'::jsonb
WHERE dashboard_code = 'NET_PROFIT';

UPDATE bi_dashboard
  SET chart_config = chart_config || '{"available_chart_types":["bar","area","data_table"]}'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
