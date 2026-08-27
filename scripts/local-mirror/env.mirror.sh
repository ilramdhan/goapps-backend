# Point the Go services at the prod-mirror postgres.
#
#   source scripts/local-mirror/env.mirror.sh
#   cd services/finance && make run
#
# NOTHING in config.yaml needs to change. Every service already binds these
# env vars over the config file via viper (AutomaticEnv + explicit BindEnv):
#   services/finance/internal/infrastructure/config/config.go:319
#   services/iam/internal/infrastructure/config/config.go:335
#   services/ppc/internal/infrastructure/config/config.go:256   (PPC_ prefix)
#
# Open a new shell to go back to the default 3-database setup, or run
# `source scripts/local-mirror/env.mirror.sh --unset`.

if [[ "${1:-}" == "--unset" ]]; then
  unset DATABASE_HOST DATABASE_PORT DATABASE_USER DATABASE_PASSWORD DATABASE_NAME DATABASE_SSLMODE
  unset PPC_DATABASE_HOST PPC_DATABASE_PORT PPC_DATABASE_USER PPC_DATABASE_PASSWORD PPC_DATABASE_NAME PPC_DATABASE_SSLMODE
  unset FINANCE_DATABASE_URL IAM_DATABASE_URL PPC_DATABASE_URL
  echo "mirror env cleared -- services will use config.yaml defaults (5434/5435/5436)"
  return 0 2>/dev/null || exit 0
fi

# --- finance + iam: shared `goapps` database, exactly as in prod -------------
export DATABASE_HOST=localhost
export DATABASE_PORT=5433
export DATABASE_USER=postgres
export DATABASE_PASSWORD=postgres
export DATABASE_NAME=goapps
export DATABASE_SSLMODE=disable

# --- ppc: its own database (locked decision D1) ------------------------------
export PPC_DATABASE_HOST=localhost
export PPC_DATABASE_PORT=5433
export PPC_DATABASE_USER=postgres
export PPC_DATABASE_PASSWORD=postgres
export PPC_DATABASE_NAME=ppc_db
export PPC_DATABASE_SSLMODE=disable

# --- migrations -------------------------------------------------------------
# `make migrate-up` reads DATABASE_URL (Makefile uses `?=`, so exporting it
# here wins). The x-migrations-table values MUST match the prod migrate-jobs --
# finance and iam share one database and would otherwise fight over a single
# default `schema_migrations` table:
#   goapps-infra/services/finance-service/base/migrate-job.yaml:62
#   goapps-infra/services/iam-service/base/migrate-job.yaml:62
#   goapps-infra/services/ppc-service/base/migrate-job.yaml:60
export FINANCE_DATABASE_URL='postgres://postgres:postgres@localhost:5433/goapps?sslmode=disable&x-migrations-table=schema_migrations_finance'
export IAM_DATABASE_URL='postgres://postgres:postgres@localhost:5433/goapps?sslmode=disable&x-migrations-table=schema_migrations_iam'
export PPC_DATABASE_URL='postgres://postgres:postgres@localhost:5433/ppc_db?sslmode=disable&x-migrations-table=schema_migrations_ppc'

cat <<'BANNER'
mirror env active -> localhost:5433
  finance + iam : goapps   (shared public schema, as in prod)
  ppc           : ppc_db

  make run                                   # picks up DATABASE_* automatically
  make migrate-up DATABASE_URL="$FINANCE_DATABASE_URL"   # in services/finance
  make migrate-up DATABASE_URL="$IAM_DATABASE_URL"       # in services/iam
  make migrate-up DATABASE_URL="$PPC_DATABASE_URL"       # in services/ppc

  psql -h localhost -p 5433 -U postgres -d goapps        # password: postgres
BANNER
