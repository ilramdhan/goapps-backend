# GoApps Backend

Backend microservices platform for **GoApps** built with Go, gRPC, and Clean Architecture principles.

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Technology Stack](#technology-stack)
4. [Repository Structure](#repository-structure)
5. [Quick Start](#quick-start)
6. [Services](#services)
7. [Development](#development)
8. [Testing](#testing)
9. [CI/CD Pipeline](#cicd-pipeline)
10. [Deployment](#deployment)
11. [API Documentation](#api-documentation)
12. [Troubleshooting](#troubleshooting)
13. [Related Documentation](#related-documentation)

---

## Overview

This repository contains backend microservices for the GoApps platform. Each service follows Clean Architecture principles and exposes both gRPC and REST APIs.

### GoApps Ecosystem

```
goapps/
├── goapps-backend/        # 🖥️  Backend microservices (this repo)
├── goapps-infra/          # 🔧 Infrastructure as Code
├── goapps-frontend/       # 🌐 Frontend application (Next.js)
└── goapps-shared-proto/   # 📝 Protocol Buffer definitions
```

| Repository | Description | Tech Stack |
|------------|-------------|------------|
| `goapps-backend` | Microservices APIs | Go 1.24, gRPC, PostgreSQL, Redis |
| `goapps-infra` | Infrastructure as Code | Kubernetes, Kustomize, Helm, ArgoCD |
| `goapps-frontend` | Web Application | Next.js 15, React, TypeScript |
| `goapps-shared-proto` | API Contracts | Protocol Buffers, Buf |

---

## Architecture

### High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                            GoApps Backend                                     │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                         EXTERNAL CLIENTS                               │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │  │
│  │  │   Next.js   │  │   Mobile    │  │   External  │                    │  │
│  │  │  Frontend   │  │    Apps     │  │   Services  │                    │  │
│  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                    │  │
│  └─────────┼────────────────┼────────────────┼───────────────────────────┘  │
│            │                │                │                               │
│            │    gRPC-Web    │     gRPC       │    REST/gRPC                 │
│            ▼                ▼                ▼                               │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                      INGRESS / LOAD BALANCER                           │  │
│  │                   (NGINX Ingress Controller)                           │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│            │                                                                 │
│            ▼                                                                 │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                       MICROSERVICES                                    │  │
│  │                                                                        │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐        │  │
│  │  │ Finance Service │  │   IAM Service   │  │  Other Services │        │  │
│  │  │   (gRPC+REST)   │  │   (gRPC+REST)   │  │   (gRPC+REST)   │        │  │
│  │  │                 │  │                 │  │                 │        │  │
│  │  │  Port 50051     │  │  Port 50052     │  │  Port 5005x     │        │  │
│  │  │  Port 8080      │  │  Port 8081      │  │  Port 808x      │        │  │
│  │  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘        │  │
│  └───────────┼────────────────────┼────────────────────┼─────────────────┘  │
│              │                    │                    │                     │
│              ▼                    ▼                    ▼                     │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                       DATA LAYER                                       │  │
│  │                                                                        │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐   │  │
│  │  │ PostgreSQL  │  │    Redis    │  │  RabbitMQ   │  │   Oracle    │   │  │
│  │  │ (Primary)   │  │   (Cache)   │  │  (Queue)    │  │  (Legacy)   │   │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘   │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐  │
│  │                     OBSERVABILITY                                      │  │
│  │                                                                        │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                    │  │
│  │  │ Prometheus  │  │   Jaeger    │  │    Loki     │                    │  │
│  │  │  (Metrics)  │  │  (Tracing)  │  │   (Logs)    │                    │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘                    │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Clean Architecture

Each service follows Clean Architecture with 4 layers:

```mermaid
graph TB
    subgraph "Service Architecture"
        subgraph "Delivery Layer"
            GRPC[gRPC Handlers]
            HTTP[HTTP Gateway]
        end
        
        subgraph "Application Layer"
            UC[Use Cases]
            DTO[DTOs/Mappers]
        end
        
        subgraph "Domain Layer"
            ENT[Entities]
            VO[Value Objects]
            REPO[Repository Interfaces]
            ERR[Domain Errors]
        end
        
        subgraph "Infrastructure Layer"
            PG[PostgreSQL Repo]
            REDIS[Redis Cache]
            CFG[Config]
            TRACE[Tracing]
        end
    end
    
    GRPC --> UC
    HTTP --> UC
    UC --> ENT
    UC --> REPO
    PG -.-> REPO
    REDIS -.-> REPO
```

### Dependency Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DEPENDENCY DIRECTION                                │
│                                                                             │
│   External World                                                            │
│        │                                                                    │
│        ▼                                                                    │
│   ┌─────────────────┐                                                       │
│   │    Delivery     │  ← Handles HTTP/gRPC requests                        │
│   │  (grpc, http)   │  ← Depends on Application layer                      │
│   └────────┬────────┘                                                       │
│            │                                                                │
│            ▼                                                                │
│   ┌─────────────────┐                                                       │
│   │   Application   │  ← Contains business logic (use cases)               │
│   │   (use cases)   │  ← Depends on Domain layer                           │
│   └────────┬────────┘                                                       │
│            │                                                                │
│            ▼                                                                │
│   ┌─────────────────┐                                                       │
│   │     Domain      │  ← Core business entities and rules                  │
│   │(entities, repos)│  ← NO external dependencies                          │
│   └────────┬────────┘                                                       │
│            │                                                                │
│            ▼                                                                │
│   ┌─────────────────┐                                                       │
│   │ Infrastructure  │  ← Implements domain interfaces                      │
│   │(postgres, redis)│  ← Can depend on external packages                   │
│   └─────────────────┘                                                       │
│                                                                             │
│   KEY: Inner layers MUST NOT depend on outer layers                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Technology Stack

### Core

| Component | Version | Description |
|-----------|---------|-------------|
| Go | 1.24 | Programming language |
| gRPC | 1.78.0 | RPC framework |
| gRPC-Gateway | 2.27.7 | REST API from gRPC |
| Protocol Buffers | 3.x | Interface definition |
| Buf | v2 | Protobuf management |

### Database & Storage

| Component | Version | Description |
|-----------|---------|-------------|
| PostgreSQL | 18 | Primary database |
| pgx | 5.8.0 | PostgreSQL driver |
| Redis | 7 | Caching layer |
| go-redis | 9.17.3 | Redis client |

### Observability

| Component | Description |
|-----------|-------------|
| zerolog | Structured logging |
| OpenTelemetry | Distributed tracing |
| Prometheus client | Metrics exposure |

### Development

| Tool | Version | Description |
|------|---------|-------------|
| golangci-lint | v2.3.0 | Code linting (20+ linters) |
| golang-migrate | v4.18.1 | Database migrations |
| testify | 1.11.1 | Testing assertions |
| excelize | 2.8.1 | Excel file processing |

---

## Repository Structure

```
goapps-backend/
│
├── 📁 services/                     # Microservices
│   └── finance/                     # Finance service
│       ├── cmd/                     # Entry points
│       │   └── server/              # Main server
│       │       └── main.go
│       ├── internal/                # Private code
│       │   ├── domain/              # Domain layer
│       │   │   └── uom/             # UOM domain
│       │   │       ├── entity.go        # Domain entity
│       │   │       ├── value_object.go  # Value objects
│       │   │       ├── repository.go    # Repository interface
│       │   │       └── errors.go        # Domain errors
│       │   ├── application/         # Application layer
│       │   │   └── uom/             # UOM use cases
│       │   │       ├── service.go       # Business logic
│       │   │       ├── dto.go           # Data transfer objects
│       │   │       └── mapper.go        # Entity ↔ DTO mapping
│       │   ├── infrastructure/      # Infrastructure layer
│       │   │   ├── config/          # Configuration
│       │   │   ├── postgres/        # PostgreSQL implementation
│       │   │   ├── redis/           # Redis caching
│       │   │   ├── tracing/         # OpenTelemetry setup
│       │   │   └── audit/           # Audit logging
│       │   └── delivery/            # Delivery layer
│       │       ├── grpc/            # gRPC handlers
│       │       └── httpdelivery/    # HTTP gateway
│       ├── pkg/                     # Shared utilities
│       │   ├── logger/              # Logging helpers
│       │   ├── circuitbreaker/      # Circuit breaker
│       │   ├── response/            # Response helpers
│       │   └── safeconv/            # Safe type conversions
│       ├── migrations/              # Database migrations
│       │   └── postgres/
│       ├── deployments/             # Deployment configs
│       │   ├── docker-compose.yaml
│       │   └── kubernetes/
│       ├── tests/                   # Tests
│       │   ├── e2e/                 # End-to-end tests
│       │   └── loadtest/            # Load tests
│       ├── docs/                    # Service docs
│       ├── Dockerfile               # Multi-stage build
│       ├── Makefile                 # Service-specific targets
│       ├── config.yaml              # Default config
│       ├── go.mod
│       └── go.sum
│
├── 📁 gen/                          # Generated code
│   ├── common/v1/                   # Common proto types
│   ├── finance/v1/                  # Finance proto types
│   ├── openapi/                     # OpenAPI specs
│   ├── go.mod
│   └── go.sum
│
├── 📁 deploy/                       # Deployment assets
│
├── 📁 .github/                      # GitHub Actions
│   ├── workflows/
│   │   ├── finance-service.yml     # Finance CI/CD
│   │   └── release-please.yml      # Release automation
│   ├── ISSUE_TEMPLATE/
│   └── PULL_REQUEST_TEMPLATE.md
│
├── .golangci.yml                    # Linter configuration
├── .gitignore
├── Makefile                         # Root Makefile
├── README.md                        # This file
├── RULES.md                         # Development rules
├── CONTRIBUTING.md                  # Contribution guide
└── LICENSE                          # Proprietary license
```

---

## Quick Start

### Prerequisites

- **Go 1.24+** - [Download](https://go.dev/dl/)
- **Docker & Docker Compose** - For local development
- **Buf CLI** - Protocol buffer management
- **golangci-lint** - Code linting
- **golang-migrate** - Database migrations

### Install Tools

```bash
# Install Buf CLI
# See: https://buf.build/docs/installation

# Install golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.3.0

# Install golang-migrate
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### Setup Development Environment

```bash
# 1. Clone repository
git clone https://github.com/mutugading/goapps-backend.git
cd goapps-backend

# 2. Generate proto code (if needed)
make proto

# 3. Start infrastructure
cd services/finance
docker compose -f deployments/docker-compose.yaml up -d postgres redis

# 4. Run migrations
export DATABASE_URL="postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable"
make finance-migrate

# 5. Run service
make finance-run
```

### Verify Installation

```bash
# Check gRPC endpoint
grpcurl -plaintext localhost:50051 list

# Check HTTP endpoint
curl http://localhost:8080/healthz

# Check metrics
curl http://localhost:8080/metrics
```

---

## Services

### Finance Service

The Finance Service manages financial data including Units of Measure (UOM).

| Property | Value |
|----------|-------|
| **Path** | `services/finance` |
| **gRPC Port** | 50051 |
| **HTTP Port** | 8080 |
| **Metrics** | `/metrics` on HTTP port |
| **Health** | `/healthz` on HTTP port |

#### API Endpoints

**gRPC Services:**
- `UOMService.CreateUOM` - Create new unit of measure
- `UOMService.GetUOM` - Get UOM by ID
- `UOMService.ListUOMs` - List all UOMs with filtering
- `UOMService.UpdateUOM` - Update existing UOM
- `UOMService.DeleteUOM` - Soft delete UOM
- `UOMService.ImportUOMs` - Bulk import from Excel
- `UOMService.ExportUOMs` - Export to Excel

**REST Endpoints (via gRPC-Gateway):**
```
POST   /api/v1/finance/uom
GET    /api/v1/finance/uom/{id}
GET    /api/v1/finance/uom
PUT    /api/v1/finance/uom/{id}
DELETE /api/v1/finance/uom/{id}
POST   /api/v1/finance/uom/import
GET    /api/v1/finance/uom/export
```

#### Configuration

Environment variables override `config.yaml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | development | Environment (development/staging/production) |
| `DATABASE_HOST` | localhost | PostgreSQL host |
| `DATABASE_PORT` | 5434 | PostgreSQL port |
| `DATABASE_USER` | finance | Database user |
| `DATABASE_PASSWORD` | - | Database password |
| `DATABASE_NAME` | finance_db | Database name |
| `REDIS_HOST` | localhost | Redis host |
| `REDIS_PORT` | 6379 | Redis port |
| `TRACING_ENABLED` | true | Enable OpenTelemetry |
| `JAEGER_ENDPOINT` | localhost:4317 | Jaeger collector |

---

## Development

### Makefile Targets

**Root Makefile:**

```bash
make help              # Show all targets
make proto             # Generate proto code
make lint              # Run golangci-lint
make lint-fix          # Auto-fix lint issues
make test              # Run all tests
make test-coverage     # Run tests with coverage
make clean             # Clean build artifacts
```

**Finance Service:**

```bash
make finance-run       # Run service locally
make finance-build     # Build binary
make finance-migrate   # Run DB migrations
make finance-seed      # Run seeders
make finance-docker    # Build Docker image
```

### Code Generation

Proto files are in `goapps-shared-proto`. To regenerate:

```bash
# From goapps-backend root
make proto

# Or from goapps-shared-proto
cd ../goapps-shared-proto
./scripts/gen-go.sh
```

Generated code is placed in `gen/` directory.

### Database Migrations

```bash
# Create new migration
migrate create -ext sql -dir services/finance/migrations/postgres -seq add_new_table

# Run migrations
export DATABASE_URL="postgres://user:pass@host:port/db?sslmode=disable"
migrate -path services/finance/migrations/postgres -database "$DATABASE_URL" up

# Rollback last migration
migrate -path services/finance/migrations/postgres -database "$DATABASE_URL" down 1
```

---

## Testing

### Test Categories

| Type | Directory | Command | Description |
|------|-----------|---------|-------------|
| Unit | `internal/` | `go test -short ./internal/...` | Isolated component tests |
| Integration | `internal/infrastructure/` | `go test ./internal/infrastructure/...` | Database/Redis tests |
| E2E | `tests/e2e/` | `go test ./tests/e2e/...` | Full API tests |
| Load | `tests/loadtest/` | Custom | Performance tests |

### Running Tests

```bash
# Unit tests only
go test -v -race -short ./...

# All tests (requires running PostgreSQL/Redis)
go test -v -race ./...

# With coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Specific package
go test -v ./internal/domain/uom/...
```

### Integration Test Setup

Integration tests require PostgreSQL and Redis:

```bash
# Start test infrastructure
docker compose -f deployments/docker-compose.yaml up -d postgres redis

# Run migrations
migrate -path migrations/postgres \
  -database "postgres://finance:finance123@localhost:5434/finance_db_test?sslmode=disable" \
  up

# Run integration tests
INTEGRATION_TEST=true go test -v ./internal/infrastructure/...
```

---

## CI/CD Pipeline

### Workflow Overview

```mermaid
graph LR
    subgraph "CI Pipeline"
        A[Push/PR] --> B[Lint]
        A --> C[Test]
        B --> D{Pass?}
        C --> D
        D -->|Yes| E[Build]
        D -->|No| F[Fail]
        E --> G[Docker Build]
    end
    
    subgraph "CD Pipeline"
        G --> H[E2E Tests]
        H --> I[Deploy Staging]
        I --> J[Deploy Production]
    end
```

### Jobs

| Job | Trigger | Description |
|-----|---------|-------------|
| `lint` | Push/PR | golangci-lint validation |
| `test` | Push/PR | Unit + integration tests |
| `build` | After lint+test | Compile binary |
| `docker` | Push to main | Build & push to GHCR |
| `e2e` | After docker | End-to-end tests |
| `deploy-staging` | After e2e | Auto-deploy via ArgoCD |
| `deploy-production` | After staging | Manual approval required |

### Self-Hosted Runners

Deployments use self-hosted runners in VPS:

```yaml
runs-on: [self-hosted, staging, goapps-runner]
```

---

## Deployment

### Container Registry

Images are pushed to GitHub Container Registry:

```
ghcr.io/mutugading/finance-service:latest
ghcr.io/mutugading/finance-service:<sha>
ghcr.io/mutugading/finance-service:main
```

### Kubernetes

Kubernetes manifests are in `goapps-infra` repository:

```
goapps-infra/services/finance-service/
├── base/
│   ├── deployment.yaml
│   ├── service.yaml
│   └── hpa.yaml
└── overlays/
    ├── staging/
    └── production/
```

### ArgoCD

Services are auto-synced via ArgoCD:

1. Push to `main` triggers Docker build
2. New image pushed to GHCR
3. ArgoCD detects new image tag
4. Auto-sync deploys to staging
5. Manual sync deploys to production

---

## API Documentation

### OpenAPI / Swagger

OpenAPI specs are generated from proto files:

```
gen/openapi/
├── finance/v1/uom.swagger.json
└── common/v1/common.swagger.json
```

### Proto Definitions

Source proto files in `goapps-shared-proto`:

```protobuf
// finance/v1/uom.proto
service UOMService {
  rpc CreateUOM(CreateUOMRequest) returns (CreateUOMResponse);
  rpc GetUOM(GetUOMRequest) returns (GetUOMResponse);
  rpc ListUOMs(ListUOMsRequest) returns (ListUOMsResponse);
  rpc UpdateUOM(UpdateUOMRequest) returns (UpdateUOMResponse);
  rpc DeleteUOM(DeleteUOMRequest) returns (DeleteUOMResponse);
}
```

### gRPC Reflection

gRPC reflection is enabled. Use grpcurl to explore:

```bash
# List services
grpcurl -plaintext localhost:50051 list

# Describe service
grpcurl -plaintext localhost:50051 describe finance.v1.UOMService

# Call method
grpcurl -plaintext -d '{"code": "KG", "name": "Kilogram"}' \
  localhost:50051 finance.v1.UOMService/CreateUOM
```

---

## Troubleshooting

### Common Issues

#### Cannot connect to database

```bash
# Check PostgreSQL is running
docker ps | grep postgres

# Test connection
psql -h localhost -p 5434 -U finance -d finance_db

# Check logs
docker logs <postgres-container-id>
```

#### gRPC connection refused

```bash
# Check service is running
lsof -i :50051

# Check logs
tail -f /path/to/service.log

# Test with grpcurl
grpcurl -plaintext localhost:50051 list
```

#### Migration failed

```bash
# Check migration status
migrate -path migrations/postgres -database "$DATABASE_URL" version

# Force version (use with caution)
migrate -path migrations/postgres -database "$DATABASE_URL" force <version>

# Check migration files
ls -la migrations/postgres/
```

#### Lint errors

```bash
# Run with verbose output
golangci-lint run -v ./...

# Fix auto-fixable issues
golangci-lint run --fix ./...

# Check specific linter
golangci-lint run --enable=errcheck ./...
```

### Useful Commands

```bash
# Check service health
curl http://localhost:8080/healthz

# View metrics
curl http://localhost:8080/metrics | grep go_

# Tail logs (with zerolog pretty print)
go run cmd/server/main.go 2>&1 | jq

# Profile CPU
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# Profile memory
go tool pprof http://localhost:8080/debug/pprof/heap
```

---

## Related Documentation

| Document | Path | Description |
|----------|------|-------------|
| Development Rules | [RULES.md](./RULES.md) | Coding conventions |
| Contributing Guide | [CONTRIBUTING.md](./CONTRIBUTING.md) | How to contribute |
| License | [LICENSE](./LICENSE) | Proprietary license |

### Issue Templates

| Template | Description |
|----------|-------------|
| [🐛 Bug Report](.github/ISSUE_TEMPLATE/bug_report.md) | Report bugs or issues |
| [✨ Feature Request](.github/ISSUE_TEMPLATE/feature_request.md) | Request new features |
| [🚀 New Service](.github/ISSUE_TEMPLATE/new_service.md) | Request new microservice |

### External Resources

- [Go Documentation](https://go.dev/doc/)
- [gRPC Documentation](https://grpc.io/docs/)
- [Buf Documentation](https://buf.build/docs/)
- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)

---

## Support & Contact

- **Team**: GoApps Backend
- **Organization**: PT Mutu Gading Tekstil
- **Repository Issues**: [GitHub Issues](https://github.com/mutugading/goapps-backend/issues)

---

## License

This project is proprietary software. See the [LICENSE](./LICENSE) file for details.

**© 2024-2026 PT Mutu Gading Tekstil. All Rights Reserved.**
