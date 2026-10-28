-- H065 (DOWN): buang layout_config.nav_tab dari dashboard tab Workforce.
BEGIN;
UPDATE bi_dashboard
SET layout_config = NULLIF(layout_config - 'nav_tab', '{}'::jsonb)
WHERE dashboard_code IN ('HR_OT_DEPT', 'HR_OT_TREND', 'HR_OT_OVERLIMIT', 'HR_OT_EMPLOYEES');
COMMIT;
