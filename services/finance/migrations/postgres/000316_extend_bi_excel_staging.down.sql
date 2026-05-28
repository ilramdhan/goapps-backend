-- Rollback: drop the extended columns from bi_excel_staging.
BEGIN;

ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS display_value;
ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS periode_label;
ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS scenario;
ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS group_3_order;
ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS group_2_order;
ALTER TABLE bi_excel_staging DROP COLUMN IF EXISTS group_1_order;

COMMIT;
