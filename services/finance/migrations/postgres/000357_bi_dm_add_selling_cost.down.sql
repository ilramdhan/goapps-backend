BEGIN;
UPDATE bi_dashboard
SET chart_config = jsonb_set(
    chart_config,
    '{metric_filter,include_metrics}',
    '["GROSS_SALES", "NETT_SALES", "COST_PROD", "MARGIN"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';
COMMIT;
