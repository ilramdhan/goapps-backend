BEGIN;
UPDATE bi_dashboard
  SET chart_config = chart_config - 'available_chart_types'
WHERE dashboard_code IN ('EBITDA','NET_PROFIT','DELIVERY_MARGIN');
COMMIT;
