-- =============================================================================
-- 10 — Databases + extensions (as `postgres`, db `postgres`). Idempotent.
-- Run AFTER 00, BEFORE restore. Collation: glibc en_US.UTF-8 (decision G-COLLATION).
-- psql -h 127.0.0.1 -U postgres -d postgres -v ON_ERROR_STOP=1 -f 10-create-databases.sql
-- In PG15+ schema `public` is owned by pg_database_owner, i.e. by <svc>_owner here.
-- =============================================================================
\set ON_ERROR_STOP on

SELECT format('CREATE DATABASE %I OWNER %I TEMPLATE template0 ENCODING ''UTF8'' LC_COLLATE ''en_US.UTF-8'' LC_CTYPE ''en_US.UTF-8''',
              s || '_db', s || '_owner')
FROM (VALUES ('iam'), ('finance'), ('ppc')) AS v(s)
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = s || '_db')
\gexec

-- Extensions need superuser; create them in every DB so restore (as <svc>_owner) never has to.
-- pg_stat_statements: cluster-wide view, install once in `postgres` for the monitor role
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
\c iam_db
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c finance_db
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\c ppc_db
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
\echo '10-create-databases: done'
