BEGIN;
DELETE FROM bi_dashboard_group WHERE group_code IN ('FINANCE', 'SALES', 'OPERATIONS', 'HR');
COMMIT;
