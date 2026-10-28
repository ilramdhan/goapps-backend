# ADR 0001 — One database per service

- **Status**: Accepted (2026-10-28)
- **Deciders**: lead dev

## Context

On the K8s cluster one superuser role (`goapps` in prod, `stgapps` in staging) and one database `goapps` were shared by finance, iam and the finance workers (schema `public`). Only ppc had its own DB (`ppc_db`). Migration tables were distinguished only by name (`schema_migrations_{finance,iam,ppc}`) inside the same DB.

Postgres migration v2 (K8s -> VPS) found:
- ad-hoc schema `cmp` (2 recon views `cmp.v_new_route_flat[_mapped]`, created outside migrations);
- empty legacy schemas `auth`, `export`, `finance`, `hr`;
- ~51 prod ad-hoc objects not created by migrations;
- table name `audit_logs` present in both finance and iam (finance migration 000491 dropped its copy).

Services connected as superuser, so nothing stopped cross-service access. Splitting the DB required a TOC-routing tool (`split_toc.py` + `ownership.json`).

## Decision

**1 service = 1 database `<svc>_db`; roles `<svc>_owner` (migrations/seeds only, direct connection), `<svc>_app` (runtime, via PgBouncer, DML only), `<svc>_ro` (read-only group); schema `public` only; migration table `schema_migrations_<svc>`; no cross-service SQL — use gRPC.**

| Role | Purpose |
|---|---|
| `<svc>_owner` | Owns all objects; DDL; migrate/seed job only; direct TLS |
| `<svc>_app` | Runtime DML; via PgBouncer (transaction mode, `max_prepared_statements=200`) |
| `<svc>_ro` | Group, SELECT only |
| `dba_read` / `dba_write` | Human groups; write only via `SET ROLE <svc>_owner` |
| `report_ro`, `backup_ro`, `monitor` | Reporting/BI, backups (local socket), monitoring |

Details: `docs/migration-postgres/v2/roles-and-access.md` (goapps workspace docs, not in this repo).

- Extensions (`pg_trgm`, `uuid-ossp`) are pre-created by the DBA scripts (`10-create-databases.sql`), because `<svc>_owner` is not a superuser. A migration may only use `CREATE EXTENSION IF NOT EXISTS` for an extension that is already provisioned (iam `000001` does this); a new extension goes into `10-create-databases.sql` first.
- Data needed from another service goes through gRPC (existing: ppc -> finance `CostMasterLookupService`, ppc -> iam).
- Cross-DB reads for reporting only via `report_ro` / BI tooling, never via service roles.

## Consequences

Positive: smaller blast radius; least privilege; independent backup/restore/scaling; no hidden coupling; local stack == VPS.

Costs:
- No cross-DB joins: use gRPC or BI.
- Extra roles and secrets per service.
- `MAINTAIN` on new finance materialized views needs re-running `25-finance-runtime.sql`.
- Restricted IAM columns are fail-closed: new ones need `30-restricted-iam.sql` re-run.
- Finance e2e ERP lifecycle test needs its own testcontainers DB with CREATEDB (test-harness need, not runtime).

## Enforcement

| Item | Status |
|---|---|
| Migration lint (`make migration-lint`, `scripts/migration-lint.sh`, allow-list `scripts/migration-lint.allow`) + CI `.github/workflows/migration-lint.yml` | Done (P8a) |
| Local `docker-compose.v2.yaml` + `make v2-up/v2-migrate/v2-seed/v2-verify` (same SQL as the VPS, copy checked by `make v2-sync-check`) | Done (P8a) |
| Written rule in backend/root CLAUDE.md + backend/feature skills | Done (P8a) |
| VPS pg_hba + CONNECT grants + `90-verify.sql` | Done (from migration v2) |
| `new-service-db.sh <svc>` one-command provisioning (roles, DB, grants, pg_hba, `<svc>-db-secret`) for local / staging `.60` / prod `.40` | Planned (P8b, after prod cutover P6 and decision P7; touches goapps-infra/VPS) |
| Infra CI check: every service overlay uses `<svc>_db` + `<svc>_app` | Planned (P8b) |
| CODEOWNERS for `migrations/**` + DB overlays | Planned (P8b) |
| Scheduled "schemas other than public" check wired to monitoring/alerts | Planned (P8b) |
| goapps-infra CLAUDE.md §15 rule | Planned (P8b) |

## Related

In this repo:
- `CLAUDE.md` §7 Migrations and §15 "New Microservice — Database Rule"
- `CONTRIBUTING.md` "Database per Service"
- `deploy/local-v2/README.md`, `scripts/migration_lint.py`, `.github/workflows/migration-lint.yml`

In the goapps workspace docs (outside this repo):
- `docs/migration-postgres/plan-v2.md`
- `docs/migration-postgres/v2/roles-and-access.md`
- `docs/migration-postgres/v2/table-ownership.md`
- `docs/migration-postgres/v2/runtime-privilege-audit.md`
- `docs/migration-postgres/handover.md` §P8
- `docs/superpowers/state/db-guardrails-STATE.md`
