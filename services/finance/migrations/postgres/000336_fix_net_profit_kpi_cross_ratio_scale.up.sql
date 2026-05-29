-- Fix NET_PROFIT cross_ratio KPI: remove scale=100 since format=percent already multiplies by 100.
-- With scale omitted (defaults to 1 in backend), the ratio is stored as fraction (0.9833)
-- and the "percent" number_format multiplies by 100 for display (98.3%).
BEGIN;

UPDATE bi_dashboard
  SET kpi_config = jsonb_set(
    kpi_config,
    '{2}',
    '{"agg":"cross_ratio","label":"Net Profit Margin","numerator_group_1":"NET PROFIT","denominator_group_1":"EBITDA","period":"l12m","compare":"none","format":"percent","decimals":1}'::jsonb
  )
WHERE dashboard_code = 'NET_PROFIT';

COMMIT;
