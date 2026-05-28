-- Rollback: drop the target_type column from bi_excel_upload.
BEGIN;

ALTER TABLE bi_excel_upload DROP COLUMN IF EXISTS target_type;

COMMIT;
