-- H026 (HD-19): company di staging OT dan bi_hr_employee, dari VIEW_BI_OT_DAILY.COMP_ID/COMP_NAME
-- (compile PROD 2026-10-22). Nullable: baris lama terisi saat jendela ETL berikutnya atau reload manual.
BEGIN;

ALTER TABLE bi_hr_stg_ot_daily ADD COLUMN IF NOT EXISTS company_id VARCHAR(30);
ALTER TABLE bi_hr_stg_ot_daily ADD COLUMN IF NOT EXISTS company    VARCHAR(200);
ALTER TABLE bi_hr_employee     ADD COLUMN IF NOT EXISTS company_id VARCHAR(30);
ALTER TABLE bi_hr_employee     ADD COLUMN IF NOT EXISTS company    VARCHAR(200);

COMMIT;
