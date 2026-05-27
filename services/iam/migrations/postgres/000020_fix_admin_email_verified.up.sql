-- IAM Service Database Migrations
-- 000020: Fix admin user email_verified_at
--
-- The seed script originally created the admin user without setting
-- email_verified_at, which forced a spurious email-verification flow
-- on first login.  This migration back-fills the column for any user
-- that holds the SUPER_ADMIN role and still has a NULL value.

UPDATE mst_user
SET    email_verified_at = created_at,
       updated_at        = NOW(),
       updated_by        = 'migration-000020'
WHERE  email_verified_at IS NULL
  AND  user_id IN (
           SELECT ur.user_id
           FROM   user_roles ur
           JOIN   mst_role   r ON r.role_id = ur.role_id
           WHERE  r.role_code = 'SUPER_ADMIN'
       );
