-- 000002 is a no-op (see the .up.sql for why), so its rollback is too.
-- It must NOT `DROP TABLE audit_logs`: in the shared `goapps` schema that table
-- is IAM's, and dropping it here would destroy IAM's audit trail.
SELECT 1;
