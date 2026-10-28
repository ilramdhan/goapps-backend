# deploy/local-v2 — local Postgres that mirrors the VPS

Used by `docker-compose.v2.yaml` and the `make v2-*` targets in the root `Makefile`.
Rule behind it: `docs/adr/0001-one-database-per-service.md` (workspace docs) and `CLAUDE.md` §15.

| Path | What |
|---|---|
| `sql/` | **Byte-identical copy** of the VPS scripts `docs/migration-postgres/v2/sql/{00,10,20,25,30,90}-*.sql` |
| `sql/SHA256SUMS` | Checksums of that copy (drift check) |
| `init/01-v2-init.sh` | First-boot init: runs 00 (dev passwords), 10, 20 ×3, 25. 30 needs IAM tables → runs in `make v2-migrate` |
| `pgbouncer/` | Transaction pooling, `max_prepared_statements=200`, userlist = `*_app` only |
| `env/<svc>.env` | Service env overrides (`iam`, `finance`, `ppc`, `workers` = finance-worker / cost-worker / cost-orchestrator) |

## Layout (same as staging/production VPS)

| DB | Owner (migrate/seed, direct `:5440`) | Runtime (services, PgBouncer `:6440`) | Migration table |
|---|---|---|---|
| `iam_db` | `iam_owner` / `dev_iam_owner` | `iam_app` / `dev_iam_app` | `schema_migrations_iam` |
| `finance_db` | `finance_owner` / `dev_finance_owner` | `finance_app` / `dev_finance_app` | `schema_migrations_finance` |
| `ppc_db` | `ppc_owner` / `dev_ppc_owner` | `ppc_app` / `dev_ppc_app` | `schema_migrations_ppc` |

Superuser: `postgres` / `dev_postgres` on `:5440` (break-glass only, like on the VPS).
All passwords here are **dev-only**; real ones live in the password manager, never in Git.

## Quick start

```bash
docker compose up -d redis rabbitmq minio minio-init mailpit   # classic infra (unchanged)
make v2-up && make v2-migrate && make v2-seed && make v2-verify
set -a; . deploy/local-v2/env/finance.env; set +a; make -C services/finance run
```

Finance e2e through PgBouncer as the runtime role:
```bash
cd services/finance && DATABASE_URL='postgres://finance_app:dev_finance_app@localhost:6440/finance_db?sslmode=disable' make test-e2e
```
(`TestErpIntegrationLifecycle` starts its own testcontainers Postgres: set `DOCKER_HOST` for colima.)

Ports 5440/6440 do not clash with the classic per-service DBs (5434/5435/5436).

## Sync with the VPS scripts — do not fork the logic

The SQL in `sql/` must stay identical to `docs/migration-postgres/v2/sql/` in the goapps workspace.
Never edit it here. Change the VPS script first, then copy it over:

```bash
make v2-sync-check            # diff against ../docs/migration-postgres/v2/sql (workspace layout)
cp ../docs/migration-postgres/v2/sql/{00,10,20,25,30,90}-*.sql deploy/local-v2/sql/
(cd deploy/local-v2/sql && shasum -a 256 *.sql > SHA256SUMS)
```
After a sync, recreate the stack: `make v2-down V2_PURGE=1 && make v2-up && make v2-migrate`.
