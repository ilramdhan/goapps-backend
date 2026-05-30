-- Reverse migration 000351: remove ETL MV job seeds.
BEGIN;

DELETE FROM bi_job WHERE job_name IN ('ETL_MIS_MV', 'ETL_SALES_MV');

-- Restore ETL_DELIVERY_MARGIN config to its pre-351 state (kind='multi_metric').
UPDATE bi_job
SET config = '{"oracle_source":"MGTDAT.VIEW_DELIVERY_MARGIN_MGT","target_type":"SALES","source_code":"ERP_ORACLE","sign_flip_in_source":true,"periode_filter":"previous_month","kind":"multi_metric"}'::jsonb,
    updated_at = NOW()
WHERE job_name = 'ETL_DELIVERY_MARGIN';

COMMIT;
