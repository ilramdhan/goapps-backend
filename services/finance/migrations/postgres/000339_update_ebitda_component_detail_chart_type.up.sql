-- Upgrade EBITDA secondary_charts[1] from plain data_table to component_detail_table
-- so the viewer renders the 6-column MoM/YoY breakdown component.
BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts, 1}',
    (layout_config -> 'secondary_charts' -> 1)
      || '{"chart_type": "component_detail_table", "chart_config": {"group1": "EBITDA", "number_format": "currency_thousands", "decimals": 1}}'::jsonb
  )
WHERE dashboard_code = 'EBITDA';

COMMIT;
