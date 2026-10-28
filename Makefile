# goapps-backend Root Makefile
# Common targets for all services

.PHONY: help proto lint test build clean

# Default target
help:
	@echo "goapps-backend Makefile"
	@echo ""
	@echo "Proto targets:"
	@echo "  make proto              - Generate proto code for all services"
	@echo ""
	@echo "Lint targets:"
	@echo "  make lint               - Run golangci-lint for all services"
	@echo "  make lint-fix           - Run golangci-lint with auto-fix"
	@echo ""
	@echo "Test targets:"
	@echo "  make test               - Run tests for all services"
	@echo "  make test-coverage      - Run tests with coverage report"
	@echo ""
	@echo "Service-specific targets:"
	@echo "  make finance-run        - Run finance service locally"
	@echo "  make finance-build      - Build finance service binary"
	@echo "  make finance-migrate    - Run finance service migrations"
	@echo "  make finance-seed       - Run finance service seeders"
	@echo "  make finance-docker     - Build finance service Docker image"
	@echo ""
	@echo "Local DB v2 (mirrors the VPS: 1 service = 1 DB, <svc>_owner/_app roles, PgBouncer):"
	@echo "  make v2-up              - Start docker-compose.v2.yaml (Postgres 18 :5440 + PgBouncer :6440)"
	@echo "  make v2-migrate         - Migrate all services as <svc>_owner (direct) + refresh grants"
	@echo "  make v2-seed            - Run seeders for all services as <svc>_owner (direct)"
	@echo "  make v2-verify          - Run 90-verify.sql on every DB"
	@echo "  make v2-down            - Stop the v2 stack (V2_PURGE=1 also drops its volume)"
	@echo "  make migration-lint     - Check migrations against the 1-DB-per-service rule"

# =============================================================================
# Proto Generation
# =============================================================================

proto:
	@echo "🔧 Generating proto code..."
	cd ../goapps-shared-proto && ./scripts/gen-go.sh
	@echo "✅ Proto generation complete"

# =============================================================================
# Linting
# =============================================================================

lint:
	@echo "🔍 Running golangci-lint..."
	golangci-lint run ./...
	@echo "✅ Lint passed"

lint-fix:
	@echo "🔧 Running golangci-lint with auto-fix..."
	golangci-lint run --fix ./...
	@echo "✅ Lint fix complete"

# =============================================================================
# Testing
# =============================================================================

test:
	@echo "🧪 Running tests..."
	go test -v -race ./...
	@echo "✅ All tests passed"

test-coverage:
	@echo "🧪 Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# =============================================================================
# Finance Service
# =============================================================================

FINANCE_DIR := services/finance

finance-run:
	@echo "🚀 Running finance service..."
	cd $(FINANCE_DIR) && go run ./cmd/server

finance-build:
	@echo "🔨 Building finance service..."
	cd $(FINANCE_DIR) && go build -o bin/finance-service ./cmd/server
	@echo "✅ Built: $(FINANCE_DIR)/bin/finance-service"

finance-migrate:
	@echo "📦 Running finance migrations..."
	cd $(FINANCE_DIR) && migrate -path migrations/postgres -database "$${DATABASE_URL}" up
	@echo "✅ Migrations applied"

finance-migrate-down:
	@echo "📦 Rolling back finance migrations..."
	cd $(FINANCE_DIR) && migrate -path migrations/postgres -database "$${DATABASE_URL}" down 1

finance-seed:
	@echo "🌱 Running finance seeders..."
	cd $(FINANCE_DIR) && go run seeds/main.go
	@echo "✅ Seeding complete"

finance-docker:
	@echo "🐳 Building finance Docker image..."
	docker build -t goapps-finance-service:latest -f $(FINANCE_DIR)/Dockerfile .
	@echo "✅ Docker image built: goapps-finance-service:latest"

finance-docker-compose:
	@echo "🐳 Starting finance service with Docker Compose..."
	docker compose -f $(FINANCE_DIR)/deployments/docker-compose.yaml up -d

finance-docker-compose-down:
	@echo "🐳 Stopping finance service Docker Compose..."
	docker compose -f $(FINANCE_DIR)/deployments/docker-compose.yaml down

# =============================================================================
# Local DB v2 — mirrors the VPS layout (docs/adr/0001-one-database-per-service.md)
# Services: source deploy/local-v2/env/<svc>.env (role <svc>_app via PgBouncer :6440).
# Migrate/seed: <svc>_owner direct on :5440. Dev passwords only: dev_<role>.
# =============================================================================

V2_COMPOSE  := docker compose -f docker-compose.v2.yaml
V2_PG       := docker exec -i goapps-v2-postgres psql -X -q -v ON_ERROR_STOP=1 -U postgres
V2_SERVICES := iam finance ppc

V2_SQL_SRC  ?= ../docs/migration-postgres/v2/sql

.PHONY: v2-up v2-down v2-migrate v2-grants v2-seed v2-verify v2-sync-check migration-lint

v2-up:
	$(V2_COMPOSE) up -d --wait
	@echo "✅ v2 up: Postgres localhost:5440, PgBouncer localhost:6440"

v2-down:
	$(V2_COMPOSE) down $(if $(V2_PURGE),-v,)

v2-migrate:
	@for s in $(V2_SERVICES); do \
	  echo "📦 $$s: migrate up as $${s}_owner"; \
	  migrate -path services/$$s/migrations/postgres -database "$$(printf 'postgres://%s_owner:dev_%s_owner@localhost:5440/%s_db?sslmode=disable&x-migrations-table=schema_migrations_%s' $$s $$s $$s $$s)" up || exit 1; \
	done
	@$(MAKE) --no-print-directory v2-grants

# Same order as the VPS after a restore/release: 20 per DB, then 25 (MV MAINTAIN) and 30 (restricted IAM).
v2-grants:
	@for s in $(V2_SERVICES); do $(V2_PG) -d $${s}_db -v svc=$$s -f /v2/sql/20-db-grants.sql || exit 1; done
	@$(V2_PG) -d finance_db -f /v2/sql/25-finance-runtime.sql
	@$(V2_PG) -d iam_db -f /v2/sql/30-restricted-iam.sql

v2-seed:
	@for s in $(V2_SERVICES); do \
	  echo "🌱 $$s: seed as $${s}_owner"; \
	  p=$$( [ $$s = ppc ] && echo PPC_ || true ); \
	  ( cd services/$$s && env $${p}DATABASE_HOST=localhost $${p}DATABASE_PORT=5440 $${p}DATABASE_USER=$${s}_owner \
	      $${p}DATABASE_PASSWORD=dev_$${s}_owner $${p}DATABASE_NAME=$${s}_db $${p}DATABASE_SSLMODE=disable \
	      go run ./seeds ) || exit 1; \
	done

v2-verify:
	@for d in iam_db finance_db ppc_db; do echo "=== $$d"; docker exec -i goapps-v2-postgres psql -X -U postgres -d $$d -f /v2/sql/90-verify.sql || exit 1; done

# deploy/local-v2/sql must stay a byte-identical copy of the VPS scripts (no forked logic).
v2-sync-check:
	@cd deploy/local-v2/sql && shasum -a 256 -c SHA256SUMS --quiet && echo "✅ local copy matches SHA256SUMS"
	@if [ -d "$(V2_SQL_SRC)" ]; then \
	  for f in deploy/local-v2/sql/*.sql; do cmp -s "$$f" "$(V2_SQL_SRC)/$$(basename $$f)" || { echo "❌ drift: $$f vs $(V2_SQL_SRC)"; exit 1; }; done; \
	  echo "✅ identical to $(V2_SQL_SRC)"; \
	else echo "ℹ️  $(V2_SQL_SRC) not found; set V2_SQL_SRC=<workspace>/docs/migration-postgres/v2/sql"; fi

migration-lint:
	@./scripts/migration-lint.sh

# =============================================================================
# Clean
# =============================================================================

clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf services/*/bin
	rm -f coverage.out coverage.html
	@echo "✅ Clean complete"
