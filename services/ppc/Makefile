# PPC (Production Planning & Control) Service Makefile
# Service-specific targets

.PHONY: help run dev build test test-unit test-integration test-e2e test-coverage \
        lint fmt vet tidy clean proto-copy-swagger \
        migrate-up migrate-down migrate-create seed \
        docker-up docker-down docker-build docker-push docker-logs \
        grpc-list grpc-health install-tools

SERVICE_NAME := ppc-service
BINARY := bin/$(SERVICE_NAME)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
DOCKER_REGISTRY ?= ghcr.io/ilramdhan
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(SERVICE_NAME)

# Auto-load .env for local development. Gitignored — copy from .env.example.
ifneq (,$(wildcard .env))
    include .env
    export
endif
ifneq (,$(wildcard .env.local))
    include .env.local
    export
endif

help:
	@echo "PPC Service Makefile"
	@echo ""
	@echo "Development:"
	@echo "  make run              - Run service locally (gRPC :50053 + HTTP :8082)"
	@echo "  make dev              - Run with hot reload (requires air)"
	@echo "  make build            - Build binary"
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

# =============================================================================
# Development
# =============================================================================

run:
	go run cmd/server/main.go

dev:
	air

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BINARY) cmd/server/main.go
	@echo "Built $(BINARY)"

# =============================================================================
# Testing
# =============================================================================

test:
	go test -v -race ./...

test-unit:
	go test -v -race -short ./internal/...

test-integration:
	INTEGRATION_TEST=true go test -v -race ./internal/infrastructure/...

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
	rm -rf bin coverage coverage.out tmp
	@echo "Cleaned build artifacts"

# =============================================================================
# Proto/Swagger
# =============================================================================

# Merge all swagger.json files from generated proto to httpdelivery for embedding.
# Run this after proto generation in goapps-shared-proto.
proto-copy-swagger:
	@mkdir -p internal/delivery/httpdelivery
	@echo "Merging PPC swagger files..."
	@if [ -d "../../gen/openapi/ppc/v1" ]; then \
		python3 ../../scripts/merge-swagger.py \
			--input-dir '../../gen/openapi/ppc/v1/*.swagger.json' \
			--output internal/delivery/httpdelivery/swagger.json \
			--title 'PPC Service API' \
			--description 'Production Planning & Control Microservice' \
			--host 'localhost:8082'; \
	else \
		echo "Error: ../../gen/openapi/ppc/v1/ not found"; \
		echo "   Run 'buf generate' in goapps-shared-proto first"; \
		exit 1; \
	fi

# =============================================================================
# Database
# =============================================================================

DATABASE_URL ?= postgres://ppc:ppc123@localhost:5436/ppc_db?sslmode=disable

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
	grpcurl -plaintext localhost:50053 list

grpc-health:
	grpcurl -plaintext localhost:50053 grpc.health.v1.Health/Check

# =============================================================================
# Tools Installation
# =============================================================================

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
