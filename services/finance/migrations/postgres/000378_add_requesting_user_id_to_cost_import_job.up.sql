-- Add requester UUID column to cost_import_job for notification routing.
-- Nullable so existing rows are unaffected.
ALTER TABLE cost_import_job
    ADD COLUMN IF NOT EXISTS cij_requesting_user_id VARCHAR(100);
