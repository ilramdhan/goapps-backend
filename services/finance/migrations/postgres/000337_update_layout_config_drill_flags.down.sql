-- Remove drill_enabled flags from secondary chart entries in layout_config.
BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    jsonb_set(layout_config, '{secondary_charts, 0}',
      (layout_config->'secondary_charts'->0) - 'drill_enabled'),
    '{secondary_charts, 1}',
    (layout_config->'secondary_charts'->1) - 'drill_enabled'
  )
WHERE dashboard_code IN ('EBITDA', 'NET_PROFIT', 'DELIVERY_MARGIN');

COMMIT;
