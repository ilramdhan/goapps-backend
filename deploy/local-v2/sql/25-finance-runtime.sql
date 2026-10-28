-- =============================================================================
-- 25 — finance_db runtime grants beyond plain DML (source: v2/runtime-privilege-audit.md)
-- Run as `postgres` on finance_db AFTER 20-db-grants.sql, and AGAIN after any release
-- whose migrations create/recreate a materialized view (default privileges cannot
-- carry MAINTAIN). Idempotent.
-- psql -h 127.0.0.1 -U postgres -d finance_db -v ON_ERROR_STOP=1 -f 25-finance-runtime.sql
--
-- Why: BI upload commit (bi_upload_repository.go:250,253) and the MV_REFRESH job
-- (bi_refresh_dashboard_mvs(), SECURITY INVOKER) run REFRESH MATERIALIZED VIEW as
-- finance_app. PG17+ MAINTAIN grants exactly REFRESH/VACUUM/ANALYZE/REINDEX/CLUSTER/LOCK,
-- nothing else — no DDL, no ownership. Audit found NO runtime TRUNCATE, DDL, temp
-- tables, session SET, LISTEN/NOTIFY or setval; LOCK TABLE ... EXCLUSIVE (period lock)
-- is already allowed by UPDATE/DELETE; COPY needs only INSERT.
-- =============================================================================
\set ON_ERROR_STOP on
SELECT current_database() = 'finance_db' AS ok \gset
\if :ok
\else
  \echo 'ABORT: run on finance_db'
  \quit
\endif

SELECT format('GRANT MAINTAIN ON public.%I TO finance_app', c.relname)
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind = 'm'
\gexec

\echo '25-finance-runtime: MAINTAIN granted on all finance materialized views'

-- Schema `cmp` (prod only: 2 ad-hoc recon views over cost_route_* / cost_product_master,
-- carried over by user decision 2026-10-27; absent on staging -> this block is a no-op).
-- 20-db-grants.sql covers schema `public` only.
SELECT to_regnamespace('cmp') IS NOT NULL AS has_cmp \gset
\if :has_cmp
  ALTER SCHEMA cmp OWNER TO finance_owner;
  SELECT format('ALTER VIEW cmp.%I OWNER TO finance_owner', c.relname)
  FROM pg_class c WHERE c.relnamespace = 'cmp'::regnamespace AND c.relkind = 'v'
    AND pg_get_userbyid(c.relowner) <> 'finance_owner'
  \gexec
  REVOKE CREATE ON SCHEMA cmp FROM PUBLIC;
  GRANT USAGE ON SCHEMA cmp TO finance_app, finance_ro, report_ro;
  GRANT SELECT ON ALL TABLES IN SCHEMA cmp TO finance_app, finance_ro;
  ALTER DEFAULT PRIVILEGES FOR ROLE finance_owner IN SCHEMA cmp GRANT SELECT ON TABLES TO finance_app, finance_ro;
  \echo '25-finance-runtime: schema cmp granted (USAGE + SELECT to finance_app/finance_ro)'
\endif
