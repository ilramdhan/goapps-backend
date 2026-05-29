-- Upgrade NET_PROFIT secondary_charts[1] from plain data_table to monthly_detail_table
-- so the viewer renders the 4-column Month/YoY/vs-EBITDA breakdown component.
BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts, 1}',
    (layout_config -> 'secondary_charts' -> 1)
      || '{"chart_type": "monthly_detail_table", "chart_config": {"compare_code": "EBITDA", "compare_label": "EBITDA", "number_format": "currency_thousands", "decimals": 1}}'::jsonb
  )
WHERE dashboard_code = 'NET_PROFIT';

COMMIT;
