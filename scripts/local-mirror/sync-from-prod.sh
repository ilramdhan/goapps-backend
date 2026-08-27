#!/usr/bin/env bash
#
# Refresh the local prod-mirror postgres from a production dump.
#
#   ./scripts/local-mirror/sync-from-prod.sh              # dump prod, restore, anonymize
#   ./scripts/local-mirror/sync-from-prod.sh --dump-only  # only produce dump files
#   ./scripts/local-mirror/sync-from-prod.sh --from-dir DIR   # restore existing dumps
#   ./scripts/local-mirror/sync-from-prod.sh --no-anonymize   # keep raw prod data
#
# Copies TWO databases, matching the prod topology:
#   goapps  -> finance + iam (shared public schema)
#   ppc_db  -> ppc
#
# This is deliberately a per-DATABASE dump/restore, not a per-table export.
# There are ~176 tables across the three services; a table-by-table copy would
# have to get FK ordering right by hand and would drift every migration. A
# database-level custom-format dump restores in dependency order via
# `pg_restore`, carries indexes/constraints/sequences, and needs no maintenance
# when a table is added.
#
# PRODUCTION SAFETY: this only ever runs pg_dump (read-only) against prod, over
# a port-forward you control. It never writes to prod. If even read load is a
# concern, pull the nightly dump from MinIO instead -- see README.md.

set -euo pipefail

# libpq (pg_dump/psql/pg_restore) is keg-only under Homebrew and not on PATH by
# default on this machine.
if [[ -d /opt/homebrew/opt/libpq/bin ]]; then
  export PATH="/opt/homebrew/opt/libpq/bin:$PATH"
fi

# ---------------------------------------------------------------------------
# Config -- override via environment
# ---------------------------------------------------------------------------
# Source: a port-forward to the prod postgres. Start it in another terminal:
#   kubectl port-forward -n database svc/postgres 15432:5432
# Point at svc/postgres (not pgbouncer): pgbouncer runs in transaction pooling
# mode, which breaks pg_dump's requirement for a single consistent session.
PROD_HOST="${PROD_HOST:-localhost}"
PROD_PORT="${PROD_PORT:-15432}"
PROD_USER="${PROD_USER:-postgres}"

# Target: the mirror container from docker-compose.mirror.yaml
LOCAL_HOST="${LOCAL_HOST:-localhost}"
LOCAL_PORT="${LOCAL_PORT:-5433}"
LOCAL_USER="${LOCAL_USER:-postgres}"
LOCAL_PASSWORD="${LOCAL_PASSWORD:-postgres}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DUMP_ROOT="${DUMP_ROOT:-${SCRIPT_DIR}/dumps}"

DATABASES=(goapps ppc_db)

DUMP_ONLY=0
NO_ANONYMIZE=0
FROM_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dump-only)    DUMP_ONLY=1; shift ;;
    --no-anonymize) NO_ANONYMIZE=1; shift ;;
    --from-dir)     FROM_DIR="${2:?--from-dir needs a directory}"; shift 2 ;;
    -h|--help)      sed -n '2,25p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "$(date +%H:%M:%S)" "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[fail]\033[0m %s\n' "$*" >&2; exit 1; }

for bin in pg_dump pg_restore psql; do
  command -v "$bin" >/dev/null || die "$bin not found. Install with: brew install libpq"
done

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
log "checking local mirror on ${LOCAL_HOST}:${LOCAL_PORT}"
PGPASSWORD="$LOCAL_PASSWORD" psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" \
  -d postgres -c 'SELECT 1' >/dev/null 2>&1 \
  || die "mirror not reachable. Start it with:
    docker compose -f docker-compose.mirror.yaml up -d"

# --- GUARD 1: the target must be the disposable mirror, not a real server ----
# This script runs DROP DATABASE on the target. Before it does, the target must
# prove it is the throwaway container by carrying the sentinel table created in
# init-mirror.sql. Production has no such table, so a mistyped LOCAL_PORT or a
# port-forward that lands on 5433 aborts here instead of dropping real data.
marker=$(PGPASSWORD="$LOCAL_PASSWORD" psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" \
  -d postgres -tAc "SELECT 1 FROM public.local_mirror_marker WHERE id = 1" 2>/dev/null || true)
[[ "$marker" == "1" ]] || die "REFUSING TO WRITE: ${LOCAL_HOST}:${LOCAL_PORT} is not a local mirror.
    The sentinel table public.local_mirror_marker is missing, which means this is
    NOT the disposable container -- it may be a real database. This script drops
    databases, so it stops here.
    If this really is your mirror, recreate it:
      docker compose -f docker-compose.mirror.yaml down -v
      docker compose -f docker-compose.mirror.yaml up -d"

# --- GUARD 2: source and target must not be the same server ------------------
# Belt and braces: if PROD_* and LOCAL_* resolve to one endpoint, the restore
# phase would dump prod and then drop it. Compare the actual system identifiers
# rather than the host strings, so localhost-vs-127.0.0.1 cannot slip through.
if [[ -z "$FROM_DIR" ]]; then
  prod_sysid=$(psql -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_USER" -d postgres \
    -tAc 'SELECT system_identifier FROM pg_control_system()' 2>/dev/null || true)
  local_sysid=$(PGPASSWORD="$LOCAL_PASSWORD" psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" \
    -d postgres -tAc 'SELECT system_identifier FROM pg_control_system()' 2>/dev/null || true)
  if [[ -n "$prod_sysid" && "$prod_sysid" == "$local_sysid" ]]; then
    die "REFUSING TO RUN: source and target are the SAME postgres server
    (system_identifier ${prod_sysid}). Restoring would destroy the source.
    Check PROD_PORT=${PROD_PORT} and LOCAL_PORT=${LOCAL_PORT}."
  fi
fi

# ---------------------------------------------------------------------------
# Dump
# ---------------------------------------------------------------------------
if [[ -n "$FROM_DIR" ]]; then
  DUMP_DIR="$FROM_DIR"
  [[ -d "$DUMP_DIR" ]] || die "no such directory: $DUMP_DIR"
  log "reusing dumps in ${DUMP_DIR}"
else
  log "checking prod reachable on ${PROD_HOST}:${PROD_PORT}"
  psql -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_USER" -d postgres -c 'SELECT 1' >/dev/null 2>&1 \
    || die "prod not reachable on ${PROD_HOST}:${PROD_PORT}.
    Start a port-forward in another terminal:
      kubectl port-forward -n database svc/postgres ${PROD_PORT}:5432
    Set PGPASSWORD, or add the host to ~/.pgpass, so pg_dump can authenticate."

  DUMP_DIR="${DUMP_ROOT}/$(date +%Y%m%d_%H%M%S)"
  mkdir -p "$DUMP_DIR"

  # --- GUARD 3: bound the load this puts on production ---------------------
  # pg_dump is read-only, but it is not free: it holds ACCESS SHARE locks and
  # can generate heavy read I/O. These settings keep it from disturbing prod.
  #
  #  default_transaction_read_only=on
  #      Hard guarantee at the server level that this session cannot write,
  #      whatever the script does. A stray write becomes an error, not damage.
  #  statement_timeout=30min
  #      A dump that wedges gets killed instead of holding locks indefinitely.
  #  lock_timeout=10s
  #      THE IMPORTANT ONE. pg_dump's ACCESS SHARE lock conflicts only with
  #      ACCESS EXCLUSIVE (DDL / migrate-job). Without this, a dump waiting on a
  #      migration would queue behind it -- and every later query would queue
  #      behind the dump, which is how a read-only dump takes an app down. With
  #      it, the dump gives up after 10s and this script fails instead of prod.
  #  idle_in_transaction_session_timeout=5min
  #      Stops an abandoned dump from pinning the snapshot and blocking vacuum.
  # Scoped to the prod commands below via `env`, NOT exported: exporting it would
  # leak default_transaction_read_only=on into the local restore session, where
  # it blocks DROP/CREATE DATABASE.
  PROD_PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=30min -c lock_timeout=10s -c idle_in_transaction_session_timeout=5min"

  # Refuse to dump through PgBouncer. It runs in transaction pooling mode
  # (POOL_MODE=transaction), which cannot hold the single consistent snapshot
  # pg_dump needs -- the dump would either fail or, worse, produce a torn copy
  # while consuming pool slots the calc workers need.
  server_ver=$(PGOPTIONS="$PROD_PGOPTIONS" psql -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_USER" \
    -d postgres -tAc 'SHOW server_version' 2>/dev/null || true)
  if [[ -z "$server_ver" ]]; then
    warn "could not read server_version -- if ${PROD_PORT} is forwarded to PgBouncer,"
    warn "re-point it at svc/postgres instead."
  fi

  for db in "${DATABASES[@]}"; do
    log "dumping ${db} from prod (read-only, lock_timeout=10s)"
    # --format=custom      -> compressed, and lets pg_restore order by dependency
    # --no-owner/--no-acl  -> local roles differ from prod; matches the prod
    #                         backup cronjob's own flags
    # --serializable-deferrable omitted on purpose: it can wait on prod's long
    #                         calc transactions. A plain snapshot is consistent
    #                         enough for a dev copy.
    PGOPTIONS="$PROD_PGOPTIONS" pg_dump \
      -h "$PROD_HOST" -p "$PROD_PORT" -U "$PROD_USER" -d "$db" \
      --format=custom --no-owner --no-acl --verbose \
      --file="${DUMP_DIR}/${db}.dump" 2> "${DUMP_DIR}/${db}.dump.log" \
      || die "pg_dump failed for ${db}; see ${DUMP_DIR}/${db}.dump.log
    If the log mentions 'canceling statement due to lock timeout', a migration or
    other DDL was running on prod. Nothing was harmed -- the dump backed off on
    purpose. Wait for it to finish and re-run."
    log "  -> $(du -h "${DUMP_DIR}/${db}.dump" | cut -f1)  ${DUMP_DIR}/${db}.dump"
  done
fi

if [[ $DUMP_ONLY -eq 1 ]]; then
  log "dump-only: files in ${DUMP_DIR}"
  exit 0
fi

# ---------------------------------------------------------------------------
# Restore
# ---------------------------------------------------------------------------
export PGPASSWORD="$LOCAL_PASSWORD"
# Clear any PGOPTIONS inherited from the caller: a read-only default from the
# environment would make DROP/CREATE DATABASE below fail confusingly.
unset PGOPTIONS

for db in "${DATABASES[@]}"; do
  dump_file="${DUMP_DIR}/${db}.dump"
  [[ -f "$dump_file" ]] || die "missing dump: ${dump_file}"

  log "recreating local database ${db}"
  # DROP + CREATE rather than --clean: a plain --clean leaves behind objects the
  # dump does not know about (e.g. a table dropped in prod since the last sync),
  # which is exactly the drift this script exists to eliminate.
  psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" -d postgres -q <<SQL
SELECT pg_terminate_backend(pid) FROM pg_stat_activity
 WHERE datname = '${db}' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS "${db}";
CREATE DATABASE "${db}";
SQL

  log "restoring ${db} into local mirror"
  # --no-owner/--no-acl again: prod role grants would fail against local roles.
  # pg_restore returns non-zero on ignorable warnings (extension comments,
  # missing roles), so failures are judged by the error count, not exit code.
  if ! pg_restore \
        -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" -d "$db" \
        --no-owner --no-acl --verbose \
        "$dump_file" > "${DUMP_DIR}/${db}.restore.log" 2>&1; then
    errs=$(grep -c '^pg_restore: error' "${DUMP_DIR}/${db}.restore.log" || true)
    if [[ "$errs" -gt 0 ]]; then
      warn "${db}: ${errs} restore error(s) -- see ${DUMP_DIR}/${db}.restore.log"
      grep '^pg_restore: error' "${DUMP_DIR}/${db}.restore.log" | head -5 >&2
    else
      log "  ${db}: restored with warnings only"
    fi
  fi

  tables=$(psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" -d "$db" -tAc \
    "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'")
  log "  ${db}: ${tables} tables in public"
done

# ---------------------------------------------------------------------------
# Anonymize
# ---------------------------------------------------------------------------
if [[ $NO_ANONYMIZE -eq 1 ]]; then
  warn "skipping anonymization -- real production personal data is now on this machine"
else
  log "anonymizing personal data and credentials in goapps"
  psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" -d goapps \
    -v ON_ERROR_STOP=1 -f "${SCRIPT_DIR}/anonymize.sql" \
    || die "anonymization failed -- the local copy still holds real prod data.
    Fix the error above and re-run, or drop the database."
  log "all user passwords are now: Dev12345!"
fi

# ---------------------------------------------------------------------------
# Verify migration state matches the codebase
# ---------------------------------------------------------------------------
# This is the payoff of mirroring prod: it tells you whether prod is behind the
# migrations in your working tree, per service, using the same version tables
# the prod migrate-jobs use (x-migrations-table=schema_migrations_<service>).
log "comparing migration versions (prod copy vs local migrations/)"
repo_root="$(cd "${SCRIPT_DIR}/../.." && pwd)"

check_version() {
  local db="$1" table="$2" svc="$3"
  local applied latest
  applied=$(psql -h "$LOCAL_HOST" -p "$LOCAL_PORT" -U "$LOCAL_USER" -d "$db" -tAc \
    "SELECT version::text || CASE WHEN dirty THEN ' (DIRTY)' ELSE '' END
       FROM ${table} LIMIT 1" 2>/dev/null) || applied=""
  latest=$(find "${repo_root}/services/${svc}/migrations/postgres" -name '*.up.sql' 2>/dev/null \
    | sed 's|.*/||; s|_.*||' | sort -n | tail -1)
  latest="${latest#"${latest%%[!0]*}"}"   # strip leading zeros for comparison
  if [[ -z "$applied" ]]; then
    warn "  ${svc}: ${table} not found in ${db} (service may never have migrated here)"
  elif [[ "$applied" == "$latest" ]]; then
    log "  ${svc}: version ${applied} -- up to date with migrations/"
  else
    warn "  ${svc}: prod at ${applied}, repo has ${latest} -> run 'make migrate-up' for this service"
  fi
}

check_version goapps schema_migrations_finance finance
check_version goapps schema_migrations_iam     iam
check_version ppc_db schema_migrations_ppc     ppc

echo
log "done. Point the services at the mirror with:"
echo "    source scripts/local-mirror/env.mirror.sh"
