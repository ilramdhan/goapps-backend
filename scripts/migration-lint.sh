#!/usr/bin/env bash
# Migration lint: enforces "1 service = 1 database, schema public only, no
# cross-service SQL" for services/*/migrations/postgres/*.sql. Logic lives in
# the sibling migration_lint.py (Python 3 stdlib only). Usage:
#   scripts/migration-lint.sh [--self-test]
# Note: CREATE EXTENSION is deliberately NOT linted (DBA scripts pre-create
# extensions; IF NOT EXISTS is harmless).
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 -I "$here/migration_lint.py" "$@"
