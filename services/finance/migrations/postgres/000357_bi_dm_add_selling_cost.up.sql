-- Migration 000357: Add SELLING_COST to Delivery Margin main chart.
-- Migration 000355 added 4 metrics (GROSS_SALES, NETT_SALES, COST_PROD, MARGIN).
-- Adding SELLING_COST for 5 total lines in the Delivery Margin Trend chart.

BEGIN;

UPDATE bi_dashboard
SET chart_config = jsonb_set(
    chart_config,
    '{metric_filter,include_metrics}',
    '["GROSS_SALES", "NETT_SALES", "COST_PROD", "SELLING_COST", "MARGIN"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
