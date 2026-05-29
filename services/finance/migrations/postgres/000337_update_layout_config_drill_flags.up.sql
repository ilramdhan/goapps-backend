-- Add drill_enabled flags to secondary chart entries in layout_config.
-- Component Detail (data_table): drill_enabled=true (rows trigger group_3 drill)
-- Monthly Detail / Trend / Cross-dashboard cards: drill_enabled=false (rows do nothing)
BEGIN;

-- EBITDA: index 0 = Trend (not drillable), index 1 = Component Detail (drillable)
UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    jsonb_set(layout_config, '{secondary_charts, 0}',
      layout_config->'secondary_charts'->0 || '{"drill_enabled": false}'::jsonb),
    '{secondary_charts, 1}',
    layout_config->'secondary_charts'->1 || '{"drill_enabled": true}'::jsonb
  )
WHERE dashboard_code = 'EBITDA';

-- NET_PROFIT: index 0 = NP vs EBITDA cross-dashboard (not drillable), index 1 = Monthly Detail (not drillable)
UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    jsonb_set(layout_config, '{secondary_charts, 0}',
      layout_config->'secondary_charts'->0 || '{"drill_enabled": false}'::jsonb),
    '{secondary_charts, 1}',
    layout_config->'secondary_charts'->1 || '{"drill_enabled": false}'::jsonb
  )
WHERE dashboard_code = 'NET_PROFIT';

-- DELIVERY_MARGIN: index 0 = Margin % by Category (not drillable), index 1 = Monthly Detail (not drillable)
UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    jsonb_set(layout_config, '{secondary_charts, 0}',
      layout_config->'secondary_charts'->0 || '{"drill_enabled": false}'::jsonb),
    '{secondary_charts, 1}',
    layout_config->'secondary_charts'->1 || '{"drill_enabled": false}'::jsonb
  )
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
