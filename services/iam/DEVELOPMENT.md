# IAM Service - Development Guide

## Prerequisites

- Go 1.26+
- Docker and Docker Compose
- [migrate CLI](https://github.com/golang-migrate/migrate) (install via `make install-tools`)
- grpcurl (optional, for testing gRPC endpoints)

Install all Go tools at pinned versions:

```bash
make install-tools
```

## Quick Start

```bash
# 1. Start infrastructure (PostgreSQL, Redis, Jaeger)
make docker-up

# 2. Wait for PostgreSQL to be healthy
docker ps    # Check iam-postgres status shows "healthy"

# 3. Run database migrations
make migrate-up

# 4. Seed default data (roles, permissions, admin user)
make seed
# OR
go run ./seeds/

# 5. Run the service
make run     # gRPC :50052, HTTP :8081
# OR
make dev     # Hot reload with air
```

## Endpoints

| Type | URL | Description |
|------|-----|-------------|
| Health | http://localhost:8081/healthz | Health check |
| Swagger | http://localhost:8081/swagger/ | API documentation |
| Metrics | http://localhost:8091/metrics | Prometheus metrics |
| Jaeger | http://localhost:16687 | Distributed tracing UI |
| gRPC | localhost:50052 | gRPC endpoint |

## Default Credentials

After running the seeder:

- **Username:** `admin`
- **Password:** `admin123`
- **Email:** `admin@goapps.local`
- **Role:** SUPER_ADMIN (all permissions)

## Testing

```bash
# All tests with race detection
make test

# Unit tests only (no external dependencies)
make test-unit

# Coverage report (generates coverage/coverage.html)
make test-coverage

# Integration tests (requires running database)
make docker-up
INTEGRATION_TEST=true make test-integration

# Full CI test locally (starts DB, creates test database, runs migrations, tests)
make test-ci-local

# E2E tests (requires running service)
make run &
E2E_TEST=true make test-e2e
```

## gRPC Testing

```bash
# List all gRPC services
make grpc-list

# Health check
make grpc-health

# Login and get a token
make grpc-login

# Use grpcurl directly
grpcurl -plaintext -d '{"username": "admin", "password": "admin123", "device_info": "dev"}' \
  localhost:50052 iam.v1.AuthService/Login
```

## Database

### Connection Details (Local Development)

| Setting | Value |
|---------|-------|
| Host | localhost |
| Port | 5435 |
| Database | iam_db |
| User | iam |
| Password | iam123 |
| SSL | disabled |

Connection string: `postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable`

### Migration Commands

```bash
make migrate-up                           # Apply all pending migrations
make migrate-down                         # Rollback last migration
make migrate-create NAME=create_xxx       # Create new migration files
```

### Seeder

The seeder (`seeds/main.go`) populates initial data:

- **Roles:** SUPER_ADMIN, ADMIN, USER, VIEWER
- **Permissions:** IAM module (user, role, permission, menu, organization, audit, session) and Finance module (UOM)
- **Admin user:** username=admin with SUPER_ADMIN role
- **Role-permission assignments:** Each role gets appropriate permissions

The seeder uses `ON CONFLICT DO NOTHING`, so it is safe to run multiple times.

## Running Both IAM and Finance Services

IAM and Finance use separate Docker Compose projects with separate local databases:

| Service | Compose Project | PostgreSQL Port | DB Name | Redis Port |
|---------|----------------|-----------------|---------|------------|
| IAM | iam-infra | 5435 | iam_db | 6380 |
| Finance | finance-infra | 5433 | finance_db | 6379 |

Both can run simultaneously without port conflicts. Start each from its own service directory:

```bash
# Terminal 1: IAM
cd services/iam
make docker-up && make migrate-up && make seed && make run

# Terminal 2: Finance
cd services/finance
make docker-up && make migrate-up && make run
```

## K3s Deployment (Staging/Production)

In the K3s cluster, both services share a single PostgreSQL instance with a shared `goapps` database. Each service uses a separate schema (`auth` for IAM, `finance` for Finance).

| Setting | Local Dev | K3s (Staging/Production) |
|---------|-----------|--------------------------|
| Database | Separate per service | Shared `goapps` database |
| DB Host | localhost | postgres.database.svc.cluster.local |
| DB Port | 5435 (IAM), 5433 (Finance) | 5432 |
| Credentials | Hardcoded in docker-compose | K8s Secret (`postgres-secret`) |

### Staging Seeder

After the first deployment to staging, run the IAM seeder once:

```bash
kubectl apply -f services/iam-service/base/seed-job.yaml -n staging
kubectl logs -f job/iam-seed -n staging
```

To re-run:

```bash
kubectl delete job iam-seed -n staging --ignore-not-found
kubectl apply -f services/iam-service/base/seed-job.yaml -n staging
```

### Production

Do NOT run the seeder in production. Create initial users and roles manually via the admin UI or through a controlled, reviewed migration.

## Project Structure

```
services/iam/
├── cmd/server/          # Application entrypoint
├── deployments/         # Docker Compose for local dev
├── internal/
│   ├── domain/          # Entities, value objects, repository interfaces
│   ├── application/     # Use case handlers (Command/Query)
│   ├── delivery/
│   │   ├── grpc/        # gRPC handlers
│   │   └── httpdelivery/# HTTP gateway, Swagger UI
│   └── infrastructure/
│       ├── config/      # Configuration loading
│       └── postgres/    # Repository implementations
├── migrations/postgres/ # Database migration files
├── seeds/               # Database seeder
└── tests/e2e/           # End-to-end tests
```

## Linting

```bash
make lint    # golangci-lint (strict)
make fmt     # go fmt + goimports
make vet     # go vet
```

The linter is strict: exported types need doc comments ending with a period, no type stuttering, no builtin shadowing, and all error paths must be handled.
