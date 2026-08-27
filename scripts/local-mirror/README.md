# Local Prod-Mirror Database

Makes the local PostgreSQL match the **production topology**, then fills it with a
scrubbed copy of production data.

## Why not just export/import each table?

Three reasons a per-table copy is the wrong tool here:

1. **Scale** — ~176 tables (finance 100, iam 40, ppc 36) and growing with every
   migration. A table list is a second source of truth that silently rots.
2. **Ordering** — the schema is heavily foreign-keyed (`mst_user_detail` →
   `mst_user`, `chat_message` → `chat_thread`, …). Getting insert order right by
   hand is error-prone; `pg_restore` derives it from the dump's dependency graph.
3. **Fidelity** — a table copy moves rows but not indexes, partial unique
   indexes, check constraints, sequences, or extensions. Those are exactly what
   makes local behave like prod.

A database-level `pg_dump --format=custom` + `pg_restore` handles all three and
needs no upkeep when tables are added.

## The topology, corrected

Production is **not** one database, and local is **not** three. Verified against
the infra manifests:

| Service | Local (default compose) | Production / Staging |
|---|---|---|
| finance | `finance_db` @ `:5434` | DB **`goapps`**, schema `public` |
| iam | `iam_db` @ `:5435` | DB **`goapps`**, schema `public` — *same as finance* |
| ppc | `ppc_db` @ `:5436` | DB **`ppc_db`** (own database) |

Sources:
- `goapps-infra/services/finance-service/base/migrate-job.yaml:62` → `/goapps?…x-migrations-table=schema_migrations_finance`
- `goapps-infra/services/iam-service/base/migrate-job.yaml:62` → `/goapps?…x-migrations-table=schema_migrations_iam`
- `goapps-infra/services/ppc-service/base/migrate-job.yaml:60` → `/ppc_db?…x-migrations-table=schema_migrations_ppc`

So the target is **2 databases in 1 container**, which is what
`docker-compose.mirror.yaml` builds.

### The bug this exposes

Finance and IAM share the `public` schema in prod, and **both** create a table
named `audit_logs` with different columns:

| | finance (`000002_create_audit_logs.up.sql`) | iam (`000006_create_audit_tables.up.sql`) |
|---|---|---|
| PK | `id` | `log_id` |
| discriminator | `action` + check constraint | `event_type` + check constraint |
| extras | `request_id`, `user_agent` | `user_id` FK, `username`, `service_name` |

Both use `CREATE TABLE IF NOT EXISTS`, so **whichever service migrates first
wins and the other's DDL is silently skipped**. In production only one shape
exists, and one of the two services is writing to a table whose columns it does
not expect. The 3-database local setup can never reproduce this. This mirror
can — which is the main reason to adopt it beyond just having prod data.

## Production safety

The script never writes to production. Three guards enforce that, and each was
tested against a throwaway server standing in for prod:

**Guard 1 — target must prove it is disposable.** `sync-from-prod.sh` runs
`DROP DATABASE` on its target. Before it does, the target must carry the sentinel
table `public.local_mirror_marker`, created only by `init-mirror.sql`. Production
has no such table, so pointing the script at a real server aborts with
`REFUSING TO WRITE` before any statement runs.

**Guard 2 — source and target must differ.** The two endpoints are compared by
`pg_control_system().system_identifier`, not by hostname, so
`localhost` vs `127.0.0.1` cannot slip past. Identical server → `REFUSING TO RUN`.

**Guard 3 — bounded load on prod.** The dump session sets:

| Setting | Why |
|---|---|
| `default_transaction_read_only=on` | server-level guarantee the session cannot write |
| `lock_timeout=10s` | **the important one** — see below |
| `statement_timeout=30min` | a wedged dump is killed, not left holding locks |
| `idle_in_transaction_session_timeout=5min` | an abandoned dump cannot pin the snapshot and block vacuum |

`lock_timeout` is what prevents an outage. `pg_dump` takes `ACCESS SHARE`, which
conflicts only with `ACCESS EXCLUSIVE` (DDL, i.e. a migrate-job). Without a
timeout, a dump waiting behind a migration would itself become a queue that every
subsequent query waits on — that is how a read-only dump takes an application
down. With it, the dump gives up after 10s and the *script* fails instead of prod.

These options are scoped to the prod commands with `env`, never exported: an
exported `default_transaction_read_only=on` would leak into the local restore and
break `DROP`/`CREATE DATABASE`.

**Zero-risk alternative:** skip prod entirely and restore from the nightly MinIO
backup — see *Lower-impact alternative* below.

### Also worth knowing

- Point the port-forward at `svc/postgres`, **not** PgBouncer. Transaction
  pooling cannot hold `pg_dump`'s consistent snapshot, and the dump would eat
  pool slots the calc workers need.
- Dumps use `--format=custom --no-owner --no-acl`, the same flags as the prod
  backup cronjob.
- `--serializable-deferrable` is deliberately *not* used: it can wait on prod's
  long calc transactions. A plain snapshot is consistent enough for a dev copy.

## Setup

```bash
# 1. Start the mirror (runs alongside the existing 3-DB compose, on :5433)
docker compose -f docker-compose.mirror.yaml up -d

# 2. Port-forward prod postgres in a separate terminal.
#    Target svc/postgres, NOT pgbouncer -- pgbouncer's transaction pooling
#    breaks pg_dump's single-session snapshot.
kubectl port-forward -n database svc/postgres 15432:5432

# 3. Authenticate. Either export the password or add it to ~/.pgpass:
export PGPASSWORD='<POSTGRES_PASSWORD from the postgres-secret>'
#    kubectl get secret postgres-secret -n database -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d

# 4. Sync
./scripts/local-mirror/sync-from-prod.sh

# 5. Point the services at it
source scripts/local-mirror/env.mirror.sh
cd services/finance && make run
```

No `config.yaml` edits are needed — all three services already bind
`DATABASE_*` / `PPC_DATABASE_*` env vars over the config file via viper.

## Flags

| Command | Effect |
|---|---|
| `sync-from-prod.sh` | dump prod → restore local → anonymize → compare migration versions |
| `--dump-only` | produce dump files, restore nothing |
| `--from-dir DIR` | restore dumps already on disk (no prod access needed) |
| `--no-anonymize` | keep raw prod data — **puts real personal data on your laptop** |

Dumps land in `scripts/local-mirror/dumps/<timestamp>/` alongside
`.dump.log` / `.restore.log` for diagnosis.

## What anonymization does

`anonymize.sql` runs against `goapps` after restore. Business data (products,
costs, parameters, org hierarchy, `employee_code`) is **kept** so the copy stays
useful; personal data and credentials are removed. Every column was verified
against the IAM migrations.

| Table | Action |
|---|---|
| `mst_user` | password → `Dev12345!`, email → `user<id>@example.local`, 2FA secret cleared, lockout reset |
| `mst_user_detail` | names/phone/address/DOB/photo/`extra_data` cleared; `employee_code` kept |
| `user_sessions`, `password_reset_tokens`, `user_recovery_codes`, `api_keys` | emptied |
| `chat_message` + children, `mst_notification` | emptied |
| `audit_logs`, `chatbot_audit_log` | emptied (hold pre-scrub row snapshots in `old_data`/`new_data`) |

`username` is intentionally preserved — it is how you log in, and
`chk_username_format` constrains its shape.

**After a sync, every user's password is `Dev12345!`.**

## Lower-impact alternative: sync from backup

Production already dumps both databases three times daily to MinIO
(`goapps-infra/base/backup/cronjobs/postgres-backup.yaml`, bucket
`postgres-backups`, filenames `production_goapps_<ts>.sql.gz` and
`production_ppc_db_<ts>.sql.gz`). Pulling those puts **zero** load on the prod
database:

```bash
mc alias set prod https://<minio-endpoint> <access> <secret>
mc cp prod/postgres-backups/production_goapps_<ts>.sql.gz  /tmp/
mc cp prod/postgres-backups/production_ppc_db_<ts>.sql.gz  /tmp/
```

Those are **plain-format gzip**, not custom-format, so restore them with `psql`
rather than `pg_restore`:

```bash
gunzip -c /tmp/production_goapps_<ts>.sql.gz \
  | psql -h localhost -p 5433 -U postgres -d goapps
psql -h localhost -p 5433 -U postgres -d goapps -f scripts/local-mirror/anonymize.sql
```

## Teardown

```bash
docker compose -f docker-compose.mirror.yaml down -v   # -v also drops the data
source scripts/local-mirror/env.mirror.sh --unset      # back to the 3-DB setup
```

## Do not commit dumps

`dumps/` holds production data. Confirm it is ignored before committing.
