-- Revert 000355: restore the 2-metric config from migration 000352.
BEGIN;

UPDATE bi_dashboard
SET chart_config = jsonb_set(
    chart_config,
    '{metric_filter,include_metrics}',
    '["NETT_SALES", "MARGIN"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
