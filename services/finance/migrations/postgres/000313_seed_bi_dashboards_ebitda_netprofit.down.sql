BEGIN;
DELETE FROM bi_dashboard WHERE dashboard_code IN ('EBITDA', 'NET_PROFIT');
COMMIT;
