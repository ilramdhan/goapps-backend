BEGIN;
ALTER TABLE bi_hr_employee     DROP COLUMN IF EXISTS company;
ALTER TABLE bi_hr_employee     DROP COLUMN IF EXISTS company_id;
ALTER TABLE bi_hr_stg_ot_daily DROP COLUMN IF EXISTS company;
ALTER TABLE bi_hr_stg_ot_daily DROP COLUMN IF EXISTS company_id;
COMMIT;
