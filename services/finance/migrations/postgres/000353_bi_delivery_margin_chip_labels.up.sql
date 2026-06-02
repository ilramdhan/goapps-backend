-- Migration 000353: Add filter chip labels to DELIVERY_MARGIN dashboard config.
--
-- Observed Oracle MV data (from .note.txt): FM_METRIC_NAME = COST_PROD and others.
-- The secondary chart config (000341) references "MARGIN" and "NETT_SALES" as metric
-- names for computed_ratio. Update the monthly_detail secondary chart to use "Margin"
-- (title-case) to match the human-readable series name returned by shapeMultiMetric()
-- (staticMetricLabels maps "MARGIN" → "Margin"). This is now handled in the BFF with
-- case-insensitive matching, but also align the config for clarity.
--
-- Also set filter_chips_group1_label and filter_chips_group2_label so the viewer
-- shows configurable labels instead of hardcoded "Delivery Type" / "Category".
BEGIN;

-- 1. Add chip labels to main chart config.
UPDATE bi_dashboard
SET chart_config = chart_config
  || '{"filter_chips_group1_label": "Delivery Type", "filter_chips_group2_label": "Category"}'::jsonb
WHERE dashboard_code = 'DELIVERY_MARGIN';

-- 2. Update monthly_detail secondary chart config to use case-matched metric name.
--    The BFF now matches case-insensitively, but align the stored config too.
UPDATE bi_dashboard
SET layout_config = jsonb_set(
  layout_config,
  '{secondary_charts}',
  (
    SELECT jsonb_agg(
      CASE
        WHEN (chart->>'chart_type') = 'monthly_detail_table'
        THEN jsonb_set(chart, '{chart_config,metric_name}', '"Margin"')
        ELSE chart
      END
    )
    FROM jsonb_array_elements(layout_config->'secondary_charts') AS chart
  )
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
