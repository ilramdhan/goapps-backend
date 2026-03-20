-- Drop partial unique indexes
DROP INDEX IF EXISTS uq_user_username;
DROP INDEX IF EXISTS uq_user_email;

-- Recreate absolute UNIQUE constraints (Warning: will fail if duplicates exist in deleted rows)
ALTER TABLE mst_user ADD CONSTRAINT uq_user_username UNIQUE (username);
ALTER TABLE mst_user ADD CONSTRAINT uq_user_email UNIQUE (email);
