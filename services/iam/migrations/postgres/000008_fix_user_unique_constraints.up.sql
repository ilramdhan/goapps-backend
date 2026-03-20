-- 000008: Fix user unique constraints
-- Convert absolute unique constraints to partial unique indexes to support soft deletion

-- Drop the absolute UNIQUE constraints
ALTER TABLE mst_user DROP CONSTRAINT IF EXISTS uq_user_username;
ALTER TABLE mst_user DROP CONSTRAINT IF EXISTS uq_user_email;

-- Create partial UNIQUE indexes that ignore soft-deleted rows
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_username ON mst_user(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_user_email ON mst_user(email) WHERE deleted_at IS NULL;
