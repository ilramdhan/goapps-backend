-- IAM Service Database Migrations
-- 000017: Rollback email verification support

ALTER TABLE mst_user DROP COLUMN IF EXISTS email_verified_at;
