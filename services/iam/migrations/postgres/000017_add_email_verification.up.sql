-- IAM Service Database Migrations
-- 000017: Add email verification support
--
-- Changes: Add email_verified_at column to mst_user
-- Note: Verification OTP codes are stored in Redis (same pattern as password reset OTP)

ALTER TABLE mst_user ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMP WITH TIME ZONE;

COMMENT ON COLUMN mst_user.email_verified_at IS 'When the user verified their email; NULL means unverified';
