-- Rollback: drop bi_audit_log.
BEGIN;

DROP INDEX IF EXISTS idx_bi_audit_changed_at;
DROP TABLE IF EXISTS bi_audit_log;

COMMIT;
