-- H016 (DOWN): hapus KPI "Jam OT per Headcount" dari HR_OT_DEPT.
BEGIN;
UPDATE bi_dashboard
SET kpi_config = COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(kpi_config) e WHERE e->>'label' <> 'Jam OT per Headcount'), '[]'::jsonb)
WHERE dashboard_code = 'HR_OT_DEPT';
COMMIT;
