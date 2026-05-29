BEGIN;
UPDATE bi_dashboard
  SET kpi_config = jsonb_set(
    kpi_config,
    '{2}',
    '{"agg":"cross_ratio","label":"Net Profit Margin","numerator_group_1":"NET PROFIT","denominator_group_1":"EBITDA","scale":100,"period":"l12m","compare":"none","format":"percent","decimals":1}'::jsonb
  )
WHERE dashboard_code = 'NET_PROFIT';
COMMIT;
