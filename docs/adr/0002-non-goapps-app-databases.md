# ADR 0002 — Non-goapps app databases on the goapps Postgres VPS

- **Status**: Accepted (2026-11-20)
- **Deciders**: lead dev

## Context

The Postgres VPS (staging `192.168.0.60`, prod `192.168.0.40`) hosts only goapps service databases (ADR 0001). The first non-goapps app, Laravel `apps.mutugading.com` (identifier `mgtapps`, own team), needs a PostgreSQL database for new tables (landing page). Its legacy Oracle schemas (`mgtapps`, `mgtdat`, `mgthris`) stay in Oracle; the app keeps its Oracle connection for them. No data is migrated from Oracle.

Laravel servers: staging `192.168.0.252`, prod `192.168.0.248` (PHP-FPM `pm.max_children=150` on both).

## Decision

**A non-goapps app may use the existing VPS only as a separate database with the ADR 0001 role pattern, limited to its own source IP, and never touching goapps databases.**

| Item | Value |
|---|---|
| Database | `mgtapps_db`, schema `public` only |
| `mgtapps_owner` | DDL (`php artisan migrate`); `CONNECTION LIMIT 5`, `statement_timeout 15min`, `idle_in_transaction_session_timeout 5min` |
| `mgtapps_app` | Runtime DML; `CONNECTION LIMIT 20` staging / `30` prod, `statement_timeout 30s`, `idle_in_transaction_session_timeout 60s` |
| `mgtapps_ro` | NOLOGIN read group |
| pg_hba | `hostssl`, scram-sha-256, only `<laravel ip>/32` (staging `.252`, prod `.248`); never `192.168.0.0/24` |
| Isolation | No access to `iam_db` / `finance_db` / `ppc_db` (CONNECT revoked) |
| Standard entries | `dba_read` / `dba_write`, backup (`pg-backup.env` `DATABASES`), monitor (exporter `--include-databases` + monitor pg_hba line) |
| Data class | T2 (`sensitive-data-policy.md`); re-classify if T0/T1 data appears |
| Connection | Direct to VPS `:5432`, `sslmode=require`, no PgBouncer for now |
| Provisioning | `goapps-infra/scripts/postgres/new-service-db.sh --external --client-cidr <ip>/32` |
| Passwords | Generated on the VPS (`openssl rand -base64 32` / hidden prompt), stored in the password manager `goapps/<env>/postgres/<role>`, never in git |

Placement rule: small app of its own team -> existing VPS (grow disk/CPU/RAM if needed). Critical, large, or other-team-at-scale app -> its own Postgres cluster/VPS.

PgBouncer upgrade path: install PgBouncer on the Laravel server (`127.0.0.1:6432`, `pool_mode=transaction`, TLS to VPS `:5432`). The source IP is unchanged, so no VPS change is needed. Laravel then needs `PDO::ATTR_EMULATE_PREPARES=true`; migrations stay direct as owner. Trigger: SQLSTATE `53300` "too many connections for role", or raise the limit within the `max_connections` budget (100 staging / 200 prod).

## Consequences

Positive: no new infrastructure; goapps data unreachable from the app; same backup/monitoring/grant tooling.

Costs:
- The app shares VPS CPU/RAM/disk and `max_connections` with goapps (budget it when raising limits).
- The monitor role `CONNECTION LIMIT` and exporter include list must be updated for the new DB.
- Direct connections: a traffic spike hits the role limit instead of being pooled.
- Laravel needs `pdo_pgsql` installed on both app servers.

## Enforcement

| Item | Status |
|---|---|
| `new-service-db.sh --external --client-cidr` | In progress (goapps-infra, parallel branch) |
| `v2/roles-and-access.md` section "Non-goapps apps" | Done |
| Laravel runbook `v2/non-goapps-app-laravel.md` | Done |
| `90-verify.sql` on `mgtapps_db` after provisioning | At provisioning |

## Related

In the goapps workspace docs (outside this repo):
- `docs/migration-postgres/v2/roles-and-access.md`
- `docs/migration-postgres/v2/non-goapps-app-laravel.md`
- `docs/migration-postgres/v2/sensitive-data-policy.md`
- `docs/migration-postgres/v2/monitoring-new-service-db.md`
- `docs/migration-postgres/handover.md` (Non-goapps web apps on the same VPS)
- ADR 0001
