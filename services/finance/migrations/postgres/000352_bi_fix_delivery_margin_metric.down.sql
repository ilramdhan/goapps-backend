-- Revert 000352: restore stacked_bar chart type and remove metric_filter.
BEGIN;

UPDATE bi_dashboard
SET
  chart_type  = 'stacked_bar',
  chart_config = chart_config
    - 'metric_filter'
    || '{"available_chart_types": ["bar", "line", "data_table"]}'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
