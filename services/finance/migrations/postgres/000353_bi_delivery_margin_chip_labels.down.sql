-- Revert 000353.
BEGIN;

UPDATE bi_dashboard
SET chart_config = chart_config - 'filter_chips_group1_label' - 'filter_chips_group2_label'
WHERE dashboard_code = 'DELIVERY_MARGIN';

-- Restore monthly_detail metric_name back to "MARGIN" (uppercase).
UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN (chart->>'chart_type') = 'monthly_detail_table'
        THEN jsonb_set(chart, '{chart_config,metric_name}', '"MARGIN"')
        ELSE chart
      END
    )
    FROM jsonb_array_elements(layout_config->'secondary_charts') AS chart
  )
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
