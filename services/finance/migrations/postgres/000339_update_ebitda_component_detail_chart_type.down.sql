-- Revert EBITDA secondary_charts[1] back to plain data_table.
BEGIN;

UPDATE bi_dashboard
  SET layout_config = jsonb_set(
    layout_config,
    '{secondary_charts, 1}',
    (layout_config -> 'secondary_charts' -> 1)
      - 'chart_config'
      || '{"chart_type": "data_table"}'::jsonb
  )
WHERE dashboard_code = 'EBITDA';

COMMIT;
