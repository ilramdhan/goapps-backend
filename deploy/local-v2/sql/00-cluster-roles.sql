-- =============================================================================
-- 00 — Cluster-wide roles (run ONCE per VPS cluster as `postgres`, db `postgres`)
-- Idempotent: re-running creates missing roles; passwords of existing LOGIN roles
-- are reset to the supplied values (so the same command doubles as a rotation).
--
-- psql -h 127.0.0.1 -U postgres -d postgres -v ON_ERROR_STOP=1 \
--   -v iam_owner_pw=...  -v iam_app_pw=... \
--   -v finance_owner_pw=... -v finance_app_pw=... \
--   -v ppc_owner_pw=...  -v ppc_app_pw=... \
--   -v backup_pw=... -v monitor_pw=... -v report_pw=... \
--   -v ilham_pw=... -v indra_pw=... \
--   -f 00-cluster-roles.sql
-- Generate each password with: openssl rand -hex 24   (hex only: DSNs are built unquoted
-- by the services — spaces, quotes, backslashes or '=' would break them; audit §PgBouncer)
-- Secrets never go into Git; store them in the team password manager.
-- =============================================================================
\set ON_ERROR_STOP on
SET password_encryption = 'scram-sha-256';

-- helper: create-or-alter a role from a psql-safe spec
CREATE OR REPLACE FUNCTION pg_temp.ensure_role(r text, attrs text, pw text)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
    EXECUTE format('CREATE ROLE %I %s', r, attrs);
  ELSE
    EXECUTE format('ALTER ROLE %I %s', r, attrs);
  END IF;
  IF pw IS NOT NULL THEN
    EXECUTE format('ALTER ROLE %I PASSWORD %L', r, pw);
  END IF;
END $$;

-- ---------------------------------------------------------------- per-service
-- <svc>_owner : LOGIN, owns DB/schema/objects, used ONLY by the migrate job.
-- <svc>_app   : LOGIN, DML only, used by the running services (via PgBouncer).
-- <svc>_ro    : NOLOGIN group, SELECT on non-restricted data.
SELECT pg_temp.ensure_role('iam_owner',     'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT CONNECTION LIMIT 5',  :'iam_owner_pw');
SELECT pg_temp.ensure_role('iam_app',       'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 100', :'iam_app_pw');
SELECT pg_temp.ensure_role('iam_ro',        'NOLOGIN', NULL);
SELECT pg_temp.ensure_role('finance_owner', 'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT CONNECTION LIMIT 5',  :'finance_owner_pw');
SELECT pg_temp.ensure_role('finance_app',   'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 150', :'finance_app_pw');
SELECT pg_temp.ensure_role('finance_ro',    'NOLOGIN', NULL);
SELECT pg_temp.ensure_role('ppc_owner',     'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT CONNECTION LIMIT 5',  :'ppc_owner_pw');
SELECT pg_temp.ensure_role('ppc_app',       'LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 60',  :'ppc_app_pw');
SELECT pg_temp.ensure_role('ppc_ro',        'NOLOGIN', NULL);

-- ---------------------------------------------------------------- humans/tools
SELECT pg_temp.ensure_role('dba_read',  'NOLOGIN', NULL);
SELECT pg_temp.ensure_role('dba_write', 'NOLOGIN', NULL);
SELECT pg_temp.ensure_role('ilham',     'LOGIN INHERIT CONNECTION LIMIT 10', :'ilham_pw');
SELECT pg_temp.ensure_role('indra',     'LOGIN INHERIT CONNECTION LIMIT 10', :'indra_pw');
SELECT pg_temp.ensure_role('report_ro', 'LOGIN INHERIT CONNECTION LIMIT 10', :'report_pw');
SELECT pg_temp.ensure_role('backup_ro', 'LOGIN INHERIT CONNECTION LIMIT 4',  :'backup_pw');
SELECT pg_temp.ensure_role('monitor',   'LOGIN INHERIT CONNECTION LIMIT 3',  :'monitor_pw');

-- read: humans see every *_ro (restricted columns excluded by 30-restricted.sql)
GRANT iam_ro, finance_ro, ppc_ro TO dba_read;
-- write: owners reachable only via explicit `SET ROLE <svc>_owner;` (PG16+ grant options)
GRANT iam_owner, finance_owner, ppc_owner TO dba_write WITH INHERIT FALSE, SET TRUE;
GRANT dba_read, dba_write TO ilham, indra;
GRANT finance_ro, ppc_ro TO report_ro;
-- report_ro gets iam_ro-equivalent grants directly in 30-restricted.sql (minus chatbot log)
GRANT pg_read_all_data TO backup_ro;
GRANT pg_monitor TO monitor;

-- ---------------------------------------------------------------- guardrails
ALTER ROLE iam_app      SET idle_in_transaction_session_timeout = '5min';
ALTER ROLE finance_app  SET idle_in_transaction_session_timeout = '15min';  -- long ETL/COPY tx
ALTER ROLE ppc_app      SET idle_in_transaction_session_timeout = '5min';
ALTER ROLE iam_owner     SET lock_timeout = '10s';
ALTER ROLE finance_owner SET lock_timeout = '10s';
ALTER ROLE ppc_owner     SET lock_timeout = '10s';
ALTER ROLE report_ro SET default_transaction_read_only = on;
ALTER ROLE report_ro SET statement_timeout = '5min';
ALTER ROLE ilham     SET statement_timeout = '15min';
ALTER ROLE indra     SET statement_timeout = '15min';
ALTER ROLE ilham     SET idle_in_transaction_session_timeout = '10min';
ALTER ROLE indra     SET idle_in_transaction_session_timeout = '10min';

-- default PUBLIC privileges on the maintenance DB
REVOKE ALL ON DATABASE postgres FROM PUBLIC;
GRANT CONNECT ON DATABASE postgres TO monitor, backup_ro;  -- backup_ro: pg_dumpall --globals-only

\echo '00-cluster-roles: done'
