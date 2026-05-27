# Finance Service Makefile
# Service-specific targets

.PHONY: help run build test lint migrate seed docker

SERVICE_NAME := finance-service
BINARY := bin/$(SERVICE_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DOCKER_REGISTRY ?= ghcr.io/ilramdhan
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(SERVICE_NAME)

# Auto-load .env for local development. Gitignored — copy from .env.example.
# Variables defined here are exported to every recipe so viper picks them up.
# Looks for .env first, then .env.local for local-only overrides.
ifneq (,$(wildcard .env))
    include .env
    export
endif
ifneq (,$(wildcard .env.local))
    include .env.local
    export
endif

help:
	@echo "Finance Service Makefile"
	@echo ""
	@echo "Development:"
	@echo "  make run              - Run service locally"
	@echo "  make build            - Build binary"
	@echo "  make dev              - Run with hot reload (requires air)"
	@echo ""
	@echo "Testing:"
	@echo "  make test             - Run all tests"
	@echo "  make test-unit        - Run unit tests only"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-e2e         - Run E2E tests"
	@echo "  make test-coverage    - Run tests with coverage"
	@echo "  make lint             - Run linter"
	@echo ""
	@echo "Database:"
	@echo "  make migrate-up       - Apply migrations"
	@echo "  make migrate-down     - Rollback last migration"
	@echo "  make migrate-create   - Create new migration (NAME=xxx)"
	@echo "  make seed             - Run seeders"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-up        - Start all services"
	@echo "  make docker-down      - Stop all services"
	@echo "  make docker-build     - Build Docker image"
	@echo "  make docker-push      - Push Docker image"

# =============================================================================
# Development
# =============================================================================

run:
	go run cmd/server/main.go

run-worker:
	go run cmd/worker/main.go

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY) cmd/server/main.go
	@echo "Built $(BINARY)"

build-worker:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o bin/finance-worker cmd/worker/main.go
	@echo "Built bin/finance-worker"

dev:
	air

# =============================================================================
# Testing
# =============================================================================

test:
	go test -v -race ./...

test-unit:
	go test -v -race -short ./internal/...

test-integration:
	INTEGRATION_TEST=true go test -v -race ./internal/infrastructure/...

# Run full CI test locally (mimics GitHub Actions)
test-ci-local:
	@echo "==> Starting test database..."
	docker compose -f deployments/docker-compose.yaml up -d postgres redis
	@echo "==> Waiting for PostgreSQL to be healthy..."
	@sleep 5
	@echo "==> Creating test database..."
	docker exec finance-postgres psql -U finance -d postgres -c "DROP DATABASE IF EXISTS finance_db_test;" || true
	docker exec finance-postgres psql -U finance -d postgres -c "CREATE DATABASE finance_db_test;"
	@echo "==> Running migrations on test database..."
	migrate -path migrations/postgres -database "postgres://finance:finance123@localhost:5434/finance_db_test?sslmode=disable" up
	@echo "==> Running integration tests..."
	INTEGRATION_TEST=true \
		TEST_DB_HOST=localhost \
		TEST_DB_PORT=5434 \
		TEST_DB_USER=finance \
		TEST_DB_PASSWORD=finance123 \
		TEST_DB_NAME=finance_db_test \
		go test -v -race ./internal/infrastructure/...
	@echo "==> CI tests completed!"

test-e2e:
	E2E_TEST=true go test -v ./tests/e2e/...

test-coverage:
	@mkdir -p coverage
	go test -v -race -coverprofile=coverage/coverage.out -covermode=atomic ./...
	go tool cover -func=coverage/coverage.out | tail -1
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report: coverage/coverage.html"

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	goimports -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin coverage coverage.out
	@echo "Cleaned build artifacts"

# =============================================================================
# Proto/Swagger
# =============================================================================

# Merge all swagger.json files from generated proto to httpdelivery for embedding
# Run this after proto generation in goapps-shared-proto
proto-copy-swagger:
	@mkdir -p internal/delivery/httpdelivery
	@echo "Merging Finance swagger files..."
	@if [ -d "../../gen/openapi/finance/v1" ]; then \
		python3 ../../scripts/merge-swagger.py \
			--input-dir '../../gen/openapi/finance/v1/*.swagger.json' \
			--output internal/delivery/httpdelivery/swagger.json \
			--title 'Finance Service API' \
			--description 'Finance Microservice' \
			--host 'localhost:8080'; \
	else \
		echo "Error: ../../gen/openapi/finance/v1/ not found"; \
		echo "   Run 'buf generate' in goapps-shared-proto first"; \
		exit 1; \
	fi

# =============================================================================
# Database
# =============================================================================

DATABASE_URL ?= postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable

migrate-up:
	migrate -path migrations/postgres -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations/postgres -database "$(DATABASE_URL)" down 1

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=create_xxx"; exit 1; fi
	migrate create -ext sql -dir migrations/postgres -seq $(NAME)

seed:
	go run seeds/main.go

# =============================================================================
# Docker
# =============================================================================

docker-up:
	docker compose -f deployments/docker-compose.yaml up -d

docker-down:
	docker compose -f deployments/docker-compose.yaml down

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(VERSION) \
		-t $(DOCKER_IMAGE):latest \
		.

docker-push:
	docker push $(DOCKER_IMAGE):$(VERSION)
	docker push $(DOCKER_IMAGE):latest

docker-logs:
	docker compose -f deployments/docker-compose.yaml logs -f

# =============================================================================
# gRPC Testing
# =============================================================================

grpc-list:
	grpcurl -plaintext localhost:50051 list

grpc-health:
	grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

grpc-list-uoms:
	grpcurl -plaintext -d '{"page": 1, "page_size": 10}' localhost:50051 finance.v1.UOMService/ListUOMs

# =============================================================================
# Tools Installation
# =============================================================================

# Tool versions - pin for security and reproducibility
GOLANGCI_LINT_VERSION := v1.62.2
AIR_VERSION := v1.52.3
GOIMPORTS_VERSION := v0.28.0
GRPCURL_VERSION := v1.9.1
MIGRATE_VERSION := v4.18.1

install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/air-verse/air@$(AIR_VERSION)
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@$(GRPCURL_VERSION)
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)
	@echo "Tools installed (pinned versions)"

# =============================================================================
# Stress fixtures (S8e.1) + scenarios (S8e.2)
# =============================================================================
#
# seed-stress  — generate a synthetic 12k-product textile FG corpus tagged
#                created_by='stress_fixture'. Drives the calc-engine stress
#                scenarios in tests/stress/.
# stress-clean — atomically delete every stress_fixture-tagged row.
# test-stress-quick — smoke-run S1+S2+S3 (~minutes on a populated corpus).
# test-stress-full  — run S1..S9 with a 30-minute test budget.

.PHONY: seed-stress stress-clean test-stress-quick test-stress-full test-stress-real

PRODUCTS ?= 12000
PARAMS   ?= 150
FORMULAS ?= 30
PERIOD   ?= 202604

seed-stress:
	go run -tags=stress ./tests/stress/fixtures/seed_calc_corpus.go \
		-products=$(PRODUCTS) \
		-params=$(PARAMS) \
		-formulas=$(FORMULAS) \
		-period=$(PERIOD)

stress-clean:
	docker exec goapps-finance-postgres psql -U finance -d finance_db -c "\
		DELETE FROM cost_product_parameter        WHERE cpp_created_by = 'stress_fixture'; \
		DELETE FROM cost_product_applicable_param WHERE capp_created_by = 'stress_fixture'; \
		DELETE FROM cost_route_rm                 WHERE crm_seq_id IN (SELECT crs_seq_id FROM cost_route_seq WHERE crs_created_by = 'stress_fixture'); \
		DELETE FROM cost_route_seq                WHERE crs_created_by = 'stress_fixture'; \
		DELETE FROM cost_route_head               WHERE crh_created_by = 'stress_fixture'; \
		DELETE FROM cost_product_master           WHERE cpm_created_by = 'stress_fixture'; \
		DELETE FROM formula_param                 WHERE formula_id IN (SELECT id FROM mst_formula WHERE created_by = 'stress_fixture'); \
		DELETE FROM mst_formula                   WHERE created_by = 'stress_fixture'; \
		DELETE FROM mst_parameter                 WHERE created_by = 'stress_fixture';"

test-stress-quick:
	STRESS_TEST=true go test -tags=stress -timeout 5m ./tests/stress/... -run "S1|S2|S3"

test-stress-full:
	STRESS_TEST=true go test -tags=stress -timeout 30m ./tests/stress/...

# Headline measurement: drives the seeded corpus through the real calc engine
# in-process (wave-ordered chunks), reports per-chunk/per-product/total wall +
# fleet extrapolation. Requires `make seed-stress` first; PERIOD must match a
# period priced in cst_rm_cost (prod data = 202604). Cleans cal_job/cost rows
# and restores the formula catalog on exit; run `make stress-clean` to drop the
# corpus afterward.
test-stress-real:
	STRESS_TEST=true STRESS_PERIOD=$(PERIOD) go test -tags=stress -timeout 30m \
		-run TestStressRealRun ./tests/stress/... -v
