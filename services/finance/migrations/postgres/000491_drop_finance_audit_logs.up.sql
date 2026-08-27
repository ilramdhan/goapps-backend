-- 000491: remove the finance-shaped `audit_logs` table left behind by 000002.
--
-- 000002 has been turned into a no-op (see that file for the full reasoning), but
-- environments that already ran its old body still carry the artifacts. This
-- migration cleans them up FORWARD, and does so conditionally, because the same
-- table name means two different things depending on where you look:
--
--   finance shape (000002, the mistake): id, table_name, record_id, action,
--                                        performed_by, performed_at, ...
--   IAM shape     (iam/000006, the owner): log_id, event_type, user_id, username,
--                                          service_name, performed_at, ...
--
-- In staging and production, finance and IAM share ONE database (`goapps`) and
-- ONE `public` schema, so this migration runs against a schema where `audit_logs`
-- may well be IAM's. IAM actively reads and writes it (AuditRepository, gRPC
-- AuditService, the /iam/audit-logs UI), so touching IAM's table would destroy a
-- live audit trail. Every statement below is therefore gated on the presence of
-- the finance-only columns AND the absence of the IAM-only ones.
--
-- Finance loses nothing: its audit surface is `cost_audit_log` (000215, backing
-- /finance/audit-logs via ListCostAuditLogs), plus aud_cost_history,
-- aud_rm_cost_history, cost_param_edit_log, bi_audit_log and friends. The only
-- code that ever referenced `audit_logs` was internal/infrastructure/audit, which
-- had zero importers and never entered the server binary.
--
-- If the finance table somehow holds rows, it is RENAMED instead of dropped, so
-- that no data is destroyed by a migration. Review and drop it by hand.
--
-- NOTE for a shared database where the finance shape won: after this migration
-- removes it, IAM's 000006 still needs to create the real table. Clear IAM's
-- dirty flag (`UPDATE schema_migrations_iam SET dirty = false WHERE version = 6`)
-- and re-run the IAM migrate job.

DO $migration$
DECLARE
    has_finance_shape boolean;
    has_iam_shape     boolean;
    row_count         bigint;
BEGIN
    IF to_regclass('public.audit_logs') IS NULL THEN
        RAISE NOTICE '000491: audit_logs does not exist -- nothing to do';
        RETURN;
    END IF;

    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'audit_logs'
          AND column_name = 'performed_by'
    ) AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'audit_logs'
          AND column_name = 'action'
    )
    INTO has_finance_shape;

    SELECT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'audit_logs'
          AND column_name IN ('log_id', 'event_type')
    )
    INTO has_iam_shape;

    IF has_iam_shape THEN
        RAISE NOTICE '000491: audit_logs is IAM-shaped (log_id/event_type) -- leaving it untouched';
        RETURN;
    END IF;

    IF NOT has_finance_shape THEN
        RAISE NOTICE '000491: audit_logs matches neither known shape -- leaving it untouched';
        RETURN;
    END IF;

    -- Only the finance shape from 000002 can reach this point.
    EXECUTE 'SELECT count(*) FROM public.audit_logs' INTO row_count;

    IF row_count > 0 THEN
        IF to_regclass('public.audit_logs_finance_legacy') IS NULL THEN
            ALTER TABLE public.audit_logs RENAME TO audit_logs_finance_legacy;
            RAISE NOTICE '000491: finance audit_logs held % row(s) -- renamed to audit_logs_finance_legacy instead of dropping; review and drop manually', row_count;
        ELSE
            RAISE NOTICE '000491: finance audit_logs held % row(s) but audit_logs_finance_legacy already exists -- leaving both in place for manual review', row_count;
        END IF;
        RETURN;
    END IF;

    DROP TABLE public.audit_logs;
    RAISE NOTICE '000491: dropped empty finance-shaped audit_logs';
END
$migration$;

-- The four indexes from 000002 belong to the table and disappear with it. Drop
-- them defensively by their exact 000002 names in case a partial run left them
-- behind on nothing (a no-op when the objects are already gone).
DROP INDEX IF EXISTS public.idx_audit_logs_table_record;
DROP INDEX IF EXISTS public.idx_audit_logs_performed_at;
DROP INDEX IF EXISTS public.idx_audit_logs_performed_by;
DROP INDEX IF EXISTS public.idx_audit_logs_action;
