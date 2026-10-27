-- H012: ambang "mendekati batas" R2 (PRD §3 R2, ≥ 10 jam, bisa diatur) di config job ETL_HR_OT_DAILY.
-- Dibaca agregasi R2 dari config->>'near_limit_hours'; kalau kosong/tidak valid, default 10. Idempotent.
UPDATE bi_job
SET config = config || '{"near_limit_hours": 10}'::jsonb
WHERE job_name = 'ETL_HR_OT_DAILY' AND NOT (config ? 'near_limit_hours');
