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
| `mgtapps_db` | `mgtapps_owner` / `dev_mgtapps_owner` | `mgtapps_app` / `dev_mgtapps_app` | Laravel `migrations` (non-goapps app: no goapps migrate/seed; `make v2-ensure-dbs` creates it on an old volume) |

Superuser: `postgres` / `dev_postgres` on `:5440` (break-glass only, like on the VPS).
All passwords here are **dev-only**; real ones live in the password manager, never in Git.

## Daily workflow

```bash
docker compose up -d redis rabbitmq minio minio-init mailpit   # classic infra (unchanged)
make v2-up                           # Postgres 127.0.0.1:5440, PgBouncer :6440
make v2-env-on                       # services/<svc>/.env.local -> v2 (PgBouncer :6440, <svc>_app)
make v2-sync                         # data: weekly sanitized staging snapshot (see docs/local-db-sync.md)
#   or empty schema instead:  make v2-migrate && make v2-seed
make -C services/iam run             # and finance / ppc / finance-cost-worker / finance-cost-orchestrator as usual
make v2-env-off                      # back to the classic per-service DBs (5434/5435/5436, legacy)
```
- `v2-env-on` only writes `.env.local` files it owns (first line `# managed-by: make v2-env-on`);
  if you already have your own `services/<svc>/.env.local`, it is skipped and you append the
  `deploy/local-v2/env/<svc>.env` lines yourself. One-off without the switch:
  `set -a; . deploy/local-v2/env/finance.env; set +a; make -C services/finance run`.
- The `.env.local` files are copies: **re-run `make v2-env-on` after pulling changes to `deploy/local-v2/env/*.env`**.
- `make v2-verify` checks the stack. On macOS with colima the v2 targets auto-detect the colima socket;
  give the VM >= 100 GiB disk (`colima stop && colima start --disk 100`).
- `make v2-sync` needs your ssh public key added to account `devsync` on the staging DB server by an admin. Full guide: [`docs/local-db-sync.md`](../../docs/local-db-sync.md).

## Credentials (dev-only)

| What | Login | Where |
|---|---|---|
| Admin, read-only | `ilham` / `dev_ilham` | any `*_db` on `:5440`; write via `SET ROLE <svc>_owner;` (restricted IAM columns hidden) |
| Superuser | `postgres` / `dev_postgres` | `:5440` |
| Migrations | `<svc>_owner` / `dev_<svc>_owner` | `:5440` |
| Runtime | `<svc>_app` / `dev_<svc>_app` | PgBouncer `:6440` |
| App login after `v2-sync` | any user incl. `admin`, shared **staging** password | password manager: `goapps/staging/app-default-password` |
| App login on a freshly seeded DB | `admin` / `admin123` | `make v2-seed` (or classic) |

DB roles are not changed by `v2-sync`. 5 failed app logins lock the account; never guess passwords. Local unlock (as `postgres` on `:5440`, in `iam_db`):
`UPDATE mst_user SET failed_login_attempts=0, is_locked=false, locked_until=NULL WHERE username='admin';`

## Migrations

- Preferred: `make v2-migrate` from the repo root (all services + grants).
- Per service: `make migrate-up` / `migrate-down` in `services/<svc>` uses v2 automatically when v2 env is on
  (`DATABASE_URL` in `.env.local`: `<svc>_owner` on `:5440`, table `schema_migrations_<svc>`).
  After a migration that creates tables, run `make v2-grants`.

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
