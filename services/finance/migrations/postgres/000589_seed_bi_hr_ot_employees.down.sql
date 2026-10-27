BEGIN;
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code = 'HR_OT_EMPLOYEES');
DELETE FROM bi_dashboard WHERE dashboard_code = 'HR_OT_EMPLOYEES';
COMMIT;
