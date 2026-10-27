-- H032 (DOWN): hapus KPI "% Karyawan Lembur" dari HR_OT_DEPT dan fakta HR_OT_EMP_COUNT.
BEGIN;
UPDATE bi_dashboard
SET kpi_config = COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(kpi_config) e WHERE e->>'label' <> '% Karyawan Lembur'), '[]'::jsonb)
WHERE dashboard_code = 'HR_OT_DEPT';
DELETE FROM bi_fact_metric WHERE type = 'HR_OT_EMP_COUNT';
COMMIT;
