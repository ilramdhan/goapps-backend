-- H060 (DOWN): buang top_section HR_OT_OVERLIMIT, dashboard over-limit bulanan dan faktanya.
BEGIN;
UPDATE bi_dashboard SET layout_config = NULLIF(layout_config - 'top_section', '{}'::jsonb) WHERE dashboard_code = 'HR_OT_OVERLIMIT';
DELETE FROM bi_dashboard_role WHERE dashboard_id IN (SELECT dashboard_id FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP', 'HR_OT_OVER_NIK'));
DELETE FROM bi_dashboard WHERE dashboard_code IN ('HR_OT_OVER_MONTH', 'HR_OT_OVER_EMP', 'HR_OT_OVER_NIK');
DELETE FROM bi_fact_metric WHERE type IN ('HR_OT_OVER_WEEKS', 'HR_OT_OVER_EMP', 'HR_OT_OVER_NIK') AND periode_grain = 'MONTHLY';
COMMIT;
