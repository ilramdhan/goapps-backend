-- =============================================================================
-- 30 — Restricted data in iam_db (policy: v2/sensitive-data-policy.md §2).
-- Run as `postgres` on iam_db AFTER 20-db-grants.sql. Idempotent; re-run after
-- every IAM release that adds columns to the mixed tables (new columns stay
-- hidden from iam_ro / report_ro until granted here — fail-closed).
-- psql -h 127.0.0.1 -U postgres -d iam_db -v ON_ERROR_STOP=1 -f 30-restricted-iam.sql
-- =============================================================================
\set ON_ERROR_STOP on
SELECT current_database() = 'iam_db' AS ok \gset
\if :ok
\else
  \echo 'ABORT: run on iam_db'
  \quit
\endif

BEGIN;

-- 1) Whole tables hidden from humans/tools (T0 secrets, T1 private chat)
REVOKE ALL ON user_sessions, password_reset_tokens, user_recovery_codes,
              chat_message, chat_message_edit_history, chat_attachment
  FROM iam_ro, report_ro;

-- 2) Mixed tables: revoke table-level SELECT, grant every column except the hidden ones
CREATE OR REPLACE FUNCTION pg_temp.grant_cols_except(tbl text, hidden text[], grantee text)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE cols text;
BEGIN
  EXECUTE format('REVOKE ALL ON public.%I FROM %I', tbl, grantee);
  SELECT string_agg(quote_ident(attname), ', ' ORDER BY attnum) INTO cols
  FROM pg_attribute
  WHERE attrelid = format('public.%I', tbl)::regclass AND attnum > 0 AND NOT attisdropped
    AND attname <> ALL (hidden);
  EXECUTE format('GRANT SELECT (%s) ON public.%I TO %I', cols, tbl, grantee);
  RAISE NOTICE '% → % : hidden %', tbl, grantee, hidden;
END $$;

SELECT pg_temp.grant_cols_except(t.tbl, t.hidden, g.grantee)
FROM (VALUES
  ('mst_user',        ARRAY['password_hash','two_factor_secret','last_login_ip']),
  ('mst_user_detail', ARRAY['phone','address']),
  ('api_keys',        ARRAY['key_hash']),
  ('audit_logs',      ARRAY['old_data','new_data','ip_address','user_agent'])
) AS t(tbl, hidden)
CROSS JOIN (VALUES ('iam_ro'), ('report_ro')) AS g(grantee);

-- 3) report_ro: all other IAM tables except chatbot_audit_log (admins keep it via iam_ro)
DO $$
DECLARE r record;
BEGIN
  FOR r IN
    SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m')
      AND c.relname NOT IN ('mst_user','mst_user_detail','api_keys','audit_logs',
                            'user_sessions','password_reset_tokens','user_recovery_codes',
                            'chat_message','chat_message_edit_history','chat_attachment',
                            'chatbot_audit_log')
  LOOP
    EXECUTE format('GRANT SELECT ON public.%I TO report_ro', r.relname);
  END LOOP;
END $$;
REVOKE ALL ON chatbot_audit_log FROM report_ro;

-- 4) Future IAM tables: report_ro follows iam_ro via default privileges
ALTER DEFAULT PRIVILEGES FOR ROLE iam_owner IN SCHEMA public GRANT SELECT ON TABLES TO report_ro;

COMMIT;
\echo '30-restricted-iam: done'
