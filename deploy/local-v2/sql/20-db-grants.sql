-- =============================================================================
-- 20 — Per-database grants. Run as `postgres`, connected to the target DB,
-- AFTER restore (and safe to re-run any time; also run after any manual DDL).
--
-- psql -h 127.0.0.1 -U postgres -d finance_db -v ON_ERROR_STOP=1 -v svc=finance -f 20-db-grants.sql
-- psql ... -d iam_db -v svc=iam ... ; psql ... -d ppc_db -v svc=ppc ...
-- =============================================================================
\set ON_ERROR_STOP on
\set db    :svc '_db'
\set owner :svc '_owner'
\set app   :svc '_app'
\set ro    :svc '_ro'

SELECT current_database() = :'db' AS on_right_db \gset
\if :on_right_db
\else
  \echo 'ABORT: connected to wrong database for svc=' :svc
  \quit
\endif

-- database
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"db" TO :"app";
GRANT CONNECT ON DATABASE :"db" TO :"ro", report_ro, backup_ro, monitor;

-- schema
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO :"app", :"ro", report_ro;

-- every restored object must belong to the owner (restore runs with --role=<svc>_owner,
-- this is a safety net for anything created by postgres)
DO $$
DECLARE r record; o text := regexp_replace(current_database(), '_db$', '_owner');
BEGIN
  FOR r IN
    SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m','S','f')
      AND pg_get_userbyid(c.relowner) <> o
      AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = c.oid AND d.deptype IN ('a','i','e'))
  LOOP
    EXECUTE format('ALTER %s public.%I OWNER TO %I',
      CASE r.relkind WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' WHEN 'S' THEN 'SEQUENCE'
                     WHEN 'f' THEN 'FOREIGN TABLE' ELSE 'TABLE' END, r.relname, o);
    RAISE NOTICE 'owner fixed: %', r.relname;
  END LOOP;
  FOR r IN
    SELECT p.oid::regprocedure AS sig FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
    WHERE n.nspname = 'public' AND pg_get_userbyid(p.proowner) <> o
      AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
  LOOP
    EXECUTE format('ALTER ROUTINE %s OWNER TO %I', r.sig, o);
    RAISE NOTICE 'owner fixed: %', r.sig;
  END LOOP;
END $$ ;

-- existing objects
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO :"app";
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO :"app";
-- EXECUTE only on the service's own routines, never on extension-owned ones
-- (e.g. pg_stat_statements_reset must not be callable by the app)
SELECT format('GRANT EXECUTE ON ROUTINE %s TO %I', p.oid::regprocedure, :'app')
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'public'
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
\gexec
GRANT SELECT ON ALL TABLES IN SCHEMA public TO :"ro";
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO :"ro";

-- future objects created by migrations (migrate job logs in as <svc>_owner)
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA public
  GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA public
  GRANT SELECT ON TABLES TO :"ro";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA public
  GRANT SELECT ON SEQUENCES TO :"ro";

-- search_path pinned for app + owner (defence against search_path tricks)
ALTER ROLE :"app"   IN DATABASE :"db" SET search_path = public;
ALTER ROLE :"owner" IN DATABASE :"db" SET search_path = public;

\echo '20-db-grants: done for' :db
