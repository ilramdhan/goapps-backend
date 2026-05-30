-- Migration 000349: Set default landing_sections config for featured dashboards.
-- landing_sections in layout_config drives the new chart-section landing page:
-- each section shows an embedded chart for the featured dashboard.
BEGIN;

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  COALESCE(layout_config, '{}'::jsonb),
  '{landing_sections}',
  '[{"chart_id":"main","title":"EBITDA Breakdown","show":true}]'::jsonb
)
WHERE dashboard_code = 'EBITDA';

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  COALESCE(layout_config, '{}'::jsonb),
  '{landing_sections}',
  '[{"chart_id":"main","title":"Net Profit Trend","show":true}]'::jsonb
)
WHERE dashboard_code = 'NET_PROFIT';

UPDATE bi_dashboard
SET layout_config = jsonb_set(
  COALESCE(layout_config, '{}'::jsonb),
  '{landing_sections}',
  '[{"chart_id":"main","title":"Delivery Margin Trend","show":true}]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
