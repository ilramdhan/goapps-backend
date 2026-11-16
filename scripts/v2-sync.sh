#!/usr/bin/env bash
# v2-sync: refresh the LOCAL v2 Postgres (container goapps-v2-postgres, 127.0.0.1:5440) from the weekly
# SANITIZED developer snapshot published on the staging DB server (see docs/local-db-sync.md).
# Works with bash 3.2 (macOS), Ubuntu and WSL2. No host Postgres tools needed: restore runs in the container.
#
# Usage: scripts/v2-sync.sh [--profile slim|full|remote] [--dry-run] [--yes] [--keep-cache] [-h]
#   --dry-run     download + verify only, no restore
#   --yes         do not ask for confirmation
#   --keep-cache  keep older snapshots in the cache too (default: keep only the latest one)
# Env (or gitignored .env.devsnap in the repo root):
#   DEVSNAP_HOST (192.168.0.60)  DEVSNAP_USER (devsync)  DEVSNAP_KEY (optional ssh key path)
#   DEVSNAP_PROFILE (slim)       DEVSNAP_CACHE (${XDG_CACHE_HOME:-$HOME/.cache}/goapps/devsnap)
#   V2_PG_CONTAINER (goapps-v2-postgres)
#   V2_SYNC_SKIP_MIGRATE=1  TEST ONLY: skip migrations (grants are still applied)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ -f "$REPO_ROOT/.env.devsnap" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$REPO_ROOT/.env.devsnap"
  set +a
fi

HOST="${DEVSNAP_HOST:-192.168.0.60}"
RUSER="${DEVSNAP_USER:-devsync}"
KEY="${DEVSNAP_KEY:-}"
PROFILE="${DEVSNAP_PROFILE:-slim}"
CACHE="${DEVSNAP_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/goapps/devsnap}"
CONTAINER="${V2_PG_CONTAINER:-goapps-v2-postgres}"
SERVICES="iam finance ppc"
DRY=0; YES=0; KEEP_CACHE=0

die()  { echo "v2-sync: ERROR: $*" >&2; exit 1; }
step() { echo; echo "[$1/$TOTAL] $2"; }
TOTAL=7

while [ $# -gt 0 ]; do
  case "$1" in
    --profile) shift; PROFILE="${1:-}" ;;
    --profile=*) PROFILE="${1#--profile=}" ;;
    --dry-run) DRY=1 ;;
    --yes|-y) YES=1 ;;
    --keep-cache) KEEP_CACHE=1 ;;
    -h|--help) sed -n 2,13p "$0"; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
  shift
done
case "$PROFILE" in slim|full|remote) ;; *) die "profile must be slim, full or remote, got '$PROFILE'" ;; esac

if [ -z "${DOCKER_HOST:-}" ] && ! docker info >/dev/null 2>&1 && [ -S "$HOME/.colima/default/docker.sock" ]; then
  export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
fi

WORK="$(mktemp -d "${TMPDIR:-/tmp}/v2-sync.XXXXXX")"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT
trap 'echo; echo "v2-sync: interrupted (partial downloads are kept in the cache and will resume)" >&2; exit 130' INT TERM

# ---------------------------------------------------------------- 1. preflight
step 1 "preflight (profile=$PROFILE, source=$RUSER@$HOST)"
SHA=""
if command -v sha256sum >/dev/null 2>&1; then SHA="sha256sum"
elif command -v shasum >/dev/null 2>&1; then SHA="shasum -a 256"; fi
for t in docker ssh rsync; do command -v "$t" >/dev/null 2>&1 || die "required tool missing: $t"; done
[ -n "$SHA" ] || die "need sha256sum or shasum"

docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null | grep -q true \
  || die "container $CONTAINER is not running. Start it with: make v2-up   (Docker running? DOCKER_HOST=${DOCKER_HOST:-default})"

# ssh wrapper script: keeps key paths with spaces intact (rsync -e takes one string)
SSH_WRAP="$WORK/ssh-wrap"
{
  echo '#!/bin/sh'
  printf 'exec ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4'
  if [ -n "$KEY" ]; then printf ' -i "%s" -o IdentitiesOnly=yes' "$KEY"; fi
  echo ' "$@"'
} > "$SSH_WRAP"
chmod 700 "$SSH_WRAP"
SSH_CMD="$SSH_WRAP"
REMOTE="$RUSER@$HOST"

# rsync: prefer GNU rsync >= 3 (resumable --append-verify); macOS ships openrsync
RSYNC=""
for c in "$(command -v rsync 2>/dev/null || true)" /opt/homebrew/bin/rsync /usr/local/bin/rsync; do
  [ -n "$c" ] && [ -x "$c" ] || continue
  if "$c" --version 2>&1 | head -n1 | grep -qi openrsync; then continue; fi
  RSYNC="$c"; break
done
if [ -n "$RSYNC" ]; then
  RSYNC_FLAGS="--partial --append-verify --progress"
else
  RSYNC="$(command -v rsync)"
  RSYNC_FLAGS="--partial --progress"
  echo "  WARNING: GNU rsync not found (openrsync lacks --append). Install it for resumable downloads: brew install rsync"
fi

# Fetch MANIFEST + SHA256SUMS (also the reachability test)
mkdir -p "$WORK/meta"
if ! "$RSYNC" -q -e "$SSH_CMD" "$REMOTE:latest/MANIFEST" "$WORK/meta/" 2>"$WORK/rsync.err"; then
  sed 's/^/  /' "$WORK/rsync.err" >&2 || true
  if grep -qi "permission denied" "$WORK/rsync.err"; then
    die "permission denied (publickey): your key is not authorised. Send your PUBLIC key to the admin (DEVSNAP_KEY=path to pick another key)."
  fi
  die "cannot fetch snapshot from $HOST. Are you on the office network or VPN? Is the snapshot built yet (Sunday ~09:30 WIB)?"
fi

# ---------------------------------------------------------------- 2. manifest
step 2 "read MANIFEST"
MAN="$WORK/meta/MANIFEST"
SRC_RUN="$(sed -n 's/^source_run=//p' "$MAN" | head -n1)"
case "$SRC_RUN" in ""|*[!A-Za-z0-9._-]*) die "MANIFEST has missing/unsafe source_run '$SRC_RUN'" ;; esac
SNAP_DIR="$(sed -n 's/^dir=//p' "$MAN" | head -n1)"
case "$SNAP_DIR" in ""|*[!A-Za-z0-9._-]*) die "MANIFEST has missing/unsafe dir '$SNAP_DIR'" ;; esac
# SHA256SUMS from the real dir (not via the 'latest' symlink) so a concurrent swap cannot mix snapshots
"$RSYNC" -q -e "$SSH_CMD" "$REMOTE:$SNAP_DIR/SHA256SUMS" "$REMOTE:$SNAP_DIR/MANIFEST" "$WORK/meta/" 2>"$WORK/rsync.err" \
  || die "cannot fetch $SNAP_DIR/SHA256SUMS (snapshot replaced meanwhile? re-run)"
SRC_RUN="$(sed -n 's/^source_run=//p' "$MAN" | head -n1)"
echo "  source_run: $SRC_RUN   dir: $SNAP_DIR   created: $(sed -n 's/^created=//p' "$MAN" | head -n1)"

# Sanity gate #1: every db comment must be sanitized p9
for s in $SERVICES; do
  c="$(sed -n "s/^comment\.${s}_db=//p" "$MAN" | head -n1)"
  c3="$(echo "$c" | awk '{print $3}')"
  case "$c" in
    "sanitized p9"*) [ "$c3" = "$SRC_RUN" ] || die "REFUSING ${s}_db: comment source_run '$c3' != MANIFEST source_run '$SRC_RUN'" ;;
    *) die "REFUSING ${s}_db: MANIFEST comment is not 'sanitized p9...' (got: '$c'). Raw data must never reach a laptop." ;;
  esac
done
echo "  sanitized p9 comments: ok"

# files of the chosen profile: lines "file.<profile>=<path> <size>"
FILES=""; TOTAL_BYTES=0
while read -r path size; do
  [ -n "$path" ] || continue
  case "$path" in */*.dump) ;; *) die "unexpected file in MANIFEST: $path" ;; esac
  case "$path" in *..*|/*) die "unsafe path in MANIFEST: $path" ;; esac
  grep -q "^[0-9a-fA-F]\{64\}  *[*]\{0,1\}${path}\$" "$WORK/meta/SHA256SUMS" || die "MANIFEST file $path is not covered by SHA256SUMS"
  FILES="$FILES $path"; TOTAL_BYTES=$((TOTAL_BYTES + size))
done <<EOT
$(sed -n "s/^file\.${PROFILE}=//p" "$MAN")
EOT
[ -n "$FILES" ] || die "profile '$PROFILE' is not in this snapshot"
for s in $SERVICES; do
  case "$FILES" in *"$PROFILE/${s}_db.dump"*) ;; *) die "snapshot lacks $PROFILE/${s}_db.dump" ;; esac
done
echo "  download size: $((TOTAL_BYTES / 1048576)) MiB"

# Disk space: restored DB is larger than the compressed dump; require 3x
RUN_DIR="$CACHE/$SNAP_DIR"
mkdir -p "$RUN_DIR/$PROFILE"
FREE_KB="$(df -Pk "$RUN_DIR" | awk 'NR==2{print $4}')"
NEED_KB=$((TOTAL_BYTES * 3 / 1024))
[ "$FREE_KB" -ge "$NEED_KB" ] || die "not enough disk in $CACHE: free $((FREE_KB / 1024)) MiB, need ~$((NEED_KB / 1024)) MiB (3x snapshot). Free some space or set DEVSNAP_CACHE."

cp "$WORK/meta/MANIFEST" "$WORK/meta/SHA256SUMS" "$RUN_DIR/"

# container disk: restored data needs far more than the compressed dumps
CDATA="$(docker exec "$CONTAINER" psql -X -U postgres -Atc 'SHOW data_directory' 2>/dev/null || true)"
CFREE_KB=""
if [ -n "$CDATA" ]; then CFREE_KB="$(docker exec "$CONTAINER" df -Pk "$CDATA" 2>/dev/null | awk 'NR==2{print $4}' || true)"; fi
if [ -n "$CFREE_KB" ]; then
  # measured 2026-11-14: slim 1.5 GB zstd dump -> ~17 GB restored (indexes included), i.e. ~12x; +5 GB headroom.
  # The old databases stay until the swap commits, so this is on top of what is already used.
  CNEED_KB=$((TOTAL_BYTES * 12 / 1024 + 5 * 1024 * 1024))
  [ "$CFREE_KB" -ge "$CNEED_KB" ] || die "not enough disk inside $CONTAINER: free $((CFREE_KB / 1024)) MiB, need ~$((CNEED_KB / 1024)) MiB (12x snapshot + 5 GB). Colima: 'colima stop && colima start --disk <GB>' (grow only); Docker Desktop: Settings > Resources > Disk image size; or 'docker system prune' unused images."
fi

# ---------------------------------------------------------------- 3. download
step 3 "download (resumable; cached files that already match are skipped)"

sum_of() { # path-relative-to-RUN_DIR -> expected sha
  awk -v p="$1" '$2==p || $2=="*"p {print $1}' "$RUN_DIR/SHA256SUMS"
}
file_ok() {
  local want got
  want="$(sum_of "$1")"; [ -n "$want" ] || return 1
  [ -f "$RUN_DIR/$1" ] || return 1
  got="$($SHA "$RUN_DIR/$1" | awk '{print $1}')"
  [ "$got" = "$want" ]
}
for f in $FILES; do
  if file_ok "$f"; then echo "  cached ok: $f"; continue; fi
  echo "  fetching: $f"
  # shellcheck disable=SC2086
  "$RSYNC" -e "$SSH_CMD" $RSYNC_FLAGS "$REMOTE:$SNAP_DIR/$f" "$RUN_DIR/$PROFILE/" \
    || die "download of $f failed (VPN dropped?). Re-run to resume."
done

# ---------------------------------------------------------------- 4. verify
step 4 "verify sha256"
for f in $FILES; do
  if ! file_ok "$f"; then
    rm -f "$RUN_DIR/$f"
    die "checksum mismatch for $f (removed from cache). Re-run make v2-sync; if it persists, tell the admin."
  fi
  echo "  ok: $f"
done

# prune old snapshots
if [ "$KEEP_CACHE" -eq 0 ]; then
  for d in "$CACHE"/*/; do
    [ -d "$d" ] || continue
    [ "$(basename "$d")" = "$SNAP_DIR" ] || { echo "  prune old cache: $d"; rm -rf "$d"; }
  done
fi

if [ "$DRY" -eq 1 ]; then
  echo; echo "v2-sync: --dry-run: download + verify done ($RUN_DIR). No restore performed."
  exit 0
fi

# ---------------------------------------------------------------- 5. confirm
step 5 "confirm"
if [ "$YES" -ne 1 ]; then
  if [ -t 0 ]; then
    printf '  This REPLACES your local iam_db, finance_db and ppc_db (container %s) with snapshot %s. Continue? [y/N] ' "$CONTAINER" "$SRC_RUN"
    read -r ans
    case "$ans" in y|Y|yes|YES) ;; *) die "aborted by user" ;; esac
  else
    die "stdin is not a terminal; pass --yes to confirm the replace of local iam_db/finance_db/ppc_db"
  fi
fi

# ---------------------------------------------------------------- 6. restore
step 6 "restore all into <db>_refresh, then swap in one transaction"
loc_psql() { docker exec -i "$CONTAINER" psql -X -q -v ON_ERROR_STOP=1 -U postgres "$@"; }
db_exists() { [ "$(loc_psql -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname='$1'")" = "1" ]; }

# recovery from an interrupted earlier swap: _old without live -> rename back (never drop); both -> drop _old
for s in $SERVICES; do
  db="${s}_db"
  if db_exists "${db}_old"; then
    if db_exists "$db"; then
      loc_psql -d postgres -c "DROP DATABASE ${db}_old WITH (FORCE)"
    else
      echo "  recovering ${db}: renaming ${db}_old back"
      loc_psql -d postgres -c "ALTER DATABASE ${db}_old RENAME TO ${db}"
    fi
  fi
done

drop_refresh() {
  for x in $SERVICES; do loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${x}_db_refresh WITH (FORCE)" >/dev/null 2>&1 || true; done
}
for s in $SERVICES; do
  db="${s}_db"; new="${db}_refresh"
  echo "== $db: restore into $new"
  loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${new} WITH (FORCE)" -c "CREATE DATABASE ${new} OWNER ${s}_owner" \
    || { drop_refresh; die "cannot create $new; local databases untouched"; }
  if ! docker exec -i "$CONTAINER" pg_restore -U postgres --no-owner --no-acl --role="${s}_owner" -d "$new" \
        < "$RUN_DIR/$PROFILE/${db}.dump"; then
    drop_refresh
    die "pg_restore of $db reported errors; all _refresh databases dropped, ALL local databases untouched"
  fi
done

echo "== swap (all three in one transaction)"
SWAP_SQL="$WORK/swap.sql"
{
  for s in $SERVICES; do
    echo "ALTER DATABASE ${s}_db ALLOW_CONNECTIONS false;"
    echo "ALTER DATABASE ${s}_db_refresh ALLOW_CONNECTIONS false;"
  done
  echo "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname IN ('iam_db','finance_db','ppc_db','iam_db_refresh','finance_db_refresh','ppc_db_refresh') AND pid <> pg_backend_pid();"
  echo "BEGIN;"
  for s in $SERVICES; do echo "ALTER DATABASE ${s}_db RENAME TO ${s}_db_old;"; done
  for s in $SERVICES; do echo "ALTER DATABASE ${s}_db_refresh RENAME TO ${s}_db;"; done
  echo "COMMIT;"
  for s in $SERVICES; do echo "ALTER DATABASE ${s}_db ALLOW_CONNECTIONS true;"; done
} > "$SWAP_SQL"
if ! docker exec -i "$CONTAINER" psql -X -q -v ON_ERROR_STOP=1 -U postgres -d postgres < "$SWAP_SQL" >/dev/null; then
  # transaction rolled back: re-enable connections on whatever is live
  for s in $SERVICES; do loc_psql -d postgres -c "ALTER DATABASE ${s}_db ALLOW_CONNECTIONS true" >/dev/null 2>&1 || true; done
  drop_refresh
  die "swap failed and was rolled back; local databases unchanged"
fi
for s in $SERVICES; do loc_psql -d postgres -c "DROP DATABASE IF EXISTS ${s}_db_old WITH (FORCE)"; done

# ---------------------------------------------------------------- 7. migrate + report
step 7 "make v2-migrate (pending migrations + grants)"
if [ "${V2_SYNC_SKIP_MIGRATE:-0}" = "1" ]; then
  echo "  migrations skipped (V2_SYNC_SKIP_MIGRATE=1, test only); applying grants"
  if [ "$CONTAINER" = "goapps-v2-postgres" ]; then
    make -C "$REPO_ROOT" --no-print-directory v2-grants || die "v2-grants failed"
  else
    echo "  (custom container $CONTAINER: make v2-grants targets goapps-v2-postgres only, not run)"
  fi
else
  make -C "$REPO_ROOT" --no-print-directory v2-migrate \
    || die "restore done but v2-migrate failed (is the 'migrate' CLI installed?). Fix and run: make v2-migrate"
fi

echo
echo "schema_migrations versions:"
for s in $SERVICES; do
  v="$(loc_psql -d "${s}_db" -Atc "SELECT version || CASE WHEN dirty THEN ' (DIRTY)' ELSE '' END FROM schema_migrations_${s} LIMIT 1" 2>/dev/null || echo '?')"
  echo "  $s: $v"
done
echo "source_run: $SRC_RUN  (profile: $PROFILE)"
echo
echo "NOTE: application logins in the synced data now use the shared STAGING password"
echo "      (password manager entry: goapps/staging/app-default-password)."
