-- Rollback: revert admin email_verified_at fix.
-- This sets email_verified_at back to NULL for users touched by the up migration.
-- In practice this rollback is rarely needed — admin email should stay verified.

UPDATE mst_user
SET    email_verified_at = NULL,
       updated_at        = NOW(),
       updated_by        = 'rollback-000020'
WHERE  updated_by = 'migration-000020';
