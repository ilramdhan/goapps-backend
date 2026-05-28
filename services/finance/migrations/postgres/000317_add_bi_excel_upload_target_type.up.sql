-- Migration: persist the target FM_TYPE on the upload session header.
-- The upload session must remember which fact-metric type the file targeted so the
-- proto BiUpload.target_type round-trips through ParseUpload/CommitUpload/ListUploads.
BEGIN;

ALTER TABLE bi_excel_upload ADD COLUMN IF NOT EXISTS target_type VARCHAR(40);

COMMIT;
