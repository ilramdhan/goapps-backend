-- Bootstrap for the prod-mirror postgres container.
-- Runs ONCE, on first container start (empty data volume) via
-- /docker-entrypoint-initdb.d. POSTGRES_DB already created `goapps`; this adds
-- the second database and the schemas prod creates in its own init script.

-- ppc lives in a dedicated database in prod (locked decision D1), not in a
-- schema of `goapps`. See goapps-infra/services/ppc-service/base/createdb-job.yaml
CREATE DATABASE ppc_db;

-- Mirror of goapps-infra/base/database/postgres/configmap.yaml -> init-schemas.sql.
-- Finance and IAM tables actually land in `public` (their migrations are not
-- schema-qualified), but these schemas exist in prod, so create them here too --
-- a prod dump may reference them.
\connect goapps
CREATE SCHEMA IF NOT EXISTS export;
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS hr;
CREATE SCHEMA IF NOT EXISTS finance;

GRANT ALL PRIVILEGES ON SCHEMA export TO postgres;
GRANT ALL PRIVILEGES ON SCHEMA auth TO postgres;
GRANT ALL PRIVILEGES ON SCHEMA hr TO postgres;
GRANT ALL PRIVILEGES ON SCHEMA finance TO postgres;

-- Extensions the IAM migrations rely on (000001 creates them, but a
-- schema-only prod dump may assume they already exist).
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

\connect ppc_db
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ---------------------------------------------------------------------------
-- Safety sentinel.
-- ---------------------------------------------------------------------------
-- sync-from-prod.sh runs DROP DATABASE against its target. It refuses to touch
-- ANY server that does not carry this marker, so a mistyped LOCAL_PORT or a
-- port-forward that happens to land on 5433 can never drop a real database.
-- Production has no such table; only a container built from this file does.
\connect postgres
CREATE TABLE IF NOT EXISTS public.local_mirror_marker (
  id            int PRIMARY KEY DEFAULT 1,
  purpose       text NOT NULL DEFAULT 'goapps local prod-mirror -- DISPOSABLE',
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT local_mirror_marker_singleton CHECK (id = 1)
);
INSERT INTO public.local_mirror_marker (id) VALUES (1) ON CONFLICT DO NOTHING;
