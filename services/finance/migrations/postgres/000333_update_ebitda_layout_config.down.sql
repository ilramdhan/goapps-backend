BEGIN;
UPDATE bi_dashboard SET layout_config = NULL WHERE dashboard_code = 'EBITDA';
COMMIT;
