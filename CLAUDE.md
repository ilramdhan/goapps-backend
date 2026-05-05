# CLAUDE.md -- GoApps Backend

> Single source of truth for Claude Code working in this repository.
> Read this before making any changes.

---

## 1. Overview

Go gRPC microservice monorepo using **Clean Architecture / DDD**. Contains two services (Finance and IAM) that share generated protobuf code from `goapps-shared-proto`.

| | |
|---|---|
| Language | Go 1.24 |
| Transport | gRPC + gRPC-Gateway (REST) |
| Database | PostgreSQL 18 (pgx v5) |
| Cache | Redis 7 (go-redis v9) |
| Config | Viper + YAML |
| Logging | zerolog |
| Tracing | OpenTelemetry + Jaeger |
| Metrics | Prometheus |
| Linting | golangci-lint v2 (27 linters) |
| Testing | testify + table-driven |

---

## 2. Repository Structure

```
goapps-backend/
├── gen/                          # Generated proto code (own go.mod, DO NOT EDIT)
│   ├── common/v1/*.pb.go
│   ├── finance/v1/*.pb.go, *_grpc.pb.go, *.pb.gw.go
│   ├── iam/v1/*.pb.go, *_grpc.pb.go, *.pb.gw.go
│   └── openapi/*.swagger.json
├── services/
│   ├── finance/                  # Finance microservice (reference implementation)
│   │   ├── cmd/server/main.go
│   │   ├── config.yaml
│   │   ├── internal/
│   │   │   ├── domain/uom/          # Entity, value objects, repo interface, errors
│   │   │   ├── application/uom/     # Handlers (create, get, list, update, delete, export, import, template)
│   │   │   ├── delivery/
│   │   │   │   ├── grpc/            # gRPC server, handler, interceptors, error mapping
│   │   │   │   └── httpdelivery/    # gRPC-Gateway REST + Swagger
│   │   │   └── infrastructure/
│   │   │       ├── postgres/        # Repository implementation
│   │   │       ├── redis/           # Cache layer (nil-safe, optional)
│   │   │       ├── config/          # Viper config loading
│   │   │       ├── tracing/         # OTLP setup
│   │   │       └── audit/           # Audit log recording
│   │   ├── migrations/postgres/     # Sequential numbered SQL migrations
│   │   ├── seeds/                   # Data seeders
│   │   ├── tests/e2e/              # End-to-end tests
│   │   ├── Makefile
│   │   ├── Dockerfile
│   │   └── go.mod                  # replace gen => ../../gen
│   └── iam/                      # IAM microservice (8 domains)
│       ├── cmd/server/main.go
│       ├── config.yaml
│       ├── internal/
│       │   ├── domain/
│       │   │   ├── auth/            # Authentication
│       │   │   ├── user/            # User management
│       │   │   ├── role/            # Roles
│       │   │   ├── organization/    # Company/Division/Department/Section
│       │   │   ├── menu/            # Hierarchical menus
│       │   │   ├── session/         # Session tracking
│       │   │   ├── audit/           # Audit logs
│       │   │   └── shared/          # Shared domain types
│       │   ├── application/
│       │   │   ├── auth/            # Login, logout, 2FA, password reset
│       │   │   ├── user/            # CRUD, roles, permissions, avatar
│       │   │   ├── role/            # CRUD + permission assignment
│       │   │   ├── permission/      # CRUD + batch operations
│       │   │   ├── organization/    # Company/Division/Department/Section CRUD
│       │   │   ├── menu/            # CRUD + tree building
│       │   │   ├── session/         # Session management
│       │   │   └── audit/           # Event logging
│       │   ├── delivery/
│       │   │   ├── grpc/            # 13 handler files + interceptors
│       │   │   └── httpdelivery/    # Gateway + Swagger
│       │   └── infrastructure/
│       │       ├── postgres/        # All repo implementations
│       │       ├── redis/           # Cache + token blacklist
│       │       ├── jwt/             # JWT token service
│       │       ├── totp/            # TOTP 2FA
│       │       ├── password/        # bcrypt hashing
│       │       ├── email/           # SMTP (Mailpit in dev)
│       │       ├── storage/         # S3/MinIO file storage
│       │       ├── config/
│       │       └── tracing/
│       ├── migrations/postgres/     # 10 migration files
│       ├── seeds/
│       ├── tests/e2e/
│       ├── Makefile
│       ├── Dockerfile
│       └── go.mod
├── scripts/                      # Utility scripts (merge-swagger.py, etc.)
├── docker-compose.yaml           # Shared local dev infrastructure
├── .golangci.yml                 # Lint config (v2 format, 27 linters)
├── Makefile                      # Root-level targets
└── RULES.md                     # Detailed coding conventions
```

---

## 3. Service Ports

| Service | gRPC Port | HTTP/Gateway Port | Database Port | Database |
|---------|-----------|-------------------|---------------|----------|
| Finance | 50051 | 8080 | 5434 | `finance_db` |
| IAM | 50052 | 8081 | 5435 | `iam_db` |

Redis is shared on port 6379 (DB 0: app cache, DB 1: token blacklist).

---

## 4. Commands

All commands are run from the service directory (e.g., `cd services/finance`).

### Development

```bash
make run                # Run service (gRPC + HTTP gateway)
make dev                # Hot reload via air
make build              # Compile binary to bin/
```

### Testing

```bash
make test               # All tests with -race
make test-unit          # Unit tests only (./internal/...)
make test-integration   # Integration tests (needs running DB, INTEGRATION_TEST=true)
make test-e2e           # End-to-end tests (E2E_TEST=true)
make test-coverage      # Coverage report (HTML at coverage/coverage.html)
make test-ci-local      # Full CI: starts Docker DB, migrates, runs integration tests
```

### Code Quality

```bash
make lint               # golangci-lint run ./...
make fmt                # go fmt + goimports
make vet                # go vet
make tidy               # go mod tidy
```

### Database

```bash
make migrate-up                    # Apply all pending migrations
make migrate-down                  # Rollback last migration
make migrate-create NAME=create_x  # Create new migration pair
make seed                          # Run data seeders
```

Finance default DATABASE_URL: `postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable`
IAM default DATABASE_URL: `postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable`

### Docker (local infra)

```bash
make docker-up          # Start PostgreSQL + Redis (service-level docker-compose)
make docker-down        # Stop
make docker-logs        # Follow logs
```

Or from repo root for shared infrastructure:
```bash
docker compose up -d    # Starts all: iam-postgres, finance-postgres, redis, mailpit, jaeger, minio
```

### gRPC Testing

```bash
# Finance
make grpc-list          # List services on :50051
make grpc-health        # Health check
make grpc-list-uoms     # List UOMs with pagination

# IAM
make grpc-list          # List services on :50052
make grpc-health
make grpc-login         # Test login
```

### Swagger

```bash
make proto-copy-swagger  # Merge generated swagger.json into httpdelivery/ for embedding
```

After running the service, Swagger UI is available at the HTTP gateway port (`:8080/swagger/` for Finance, `:8081/swagger/` for IAM).

### Root Makefile (from repo root)

```bash
make proto              # Generate proto code via goapps-shared-proto
make lint               # Lint all services
make test               # Test all services
make finance-run        # Run finance from root
make finance-build      # Build finance from root
make clean              # Remove all build artifacts
```

### Tool Installation

```bash
make install-tools      # Installs pinned versions of golangci-lint, air, goimports, grpcurl, migrate
```

---

## 5. Architecture Layers

Dependencies flow **inward only**:

```
Delivery  -->  Application  -->  Domain  <--  Infrastructure
```

### Domain Layer (`internal/domain/<entity>/`)

Pure business logic. **MUST NOT import any external packages** -- standard library only.

| File | Purpose |
|------|---------|
| `entity.go` | Aggregate root with **private fields**, validated constructors (`NewXxx`), behavior methods |
| `value_objects.go` | Immutable types with validation (e.g., `Code`, `Name`, `Category`) |
| `repository.go` | Repository **interface** (contract only, no implementation) |
| `errors.go` | Sentinel errors (`var ErrNotFound = errors.New(...)`) |

```go
// Value object pattern -- private field, validated constructor, read-only getter
type Code struct{ value string }
func NewCode(s string) (Code, error) { /* validate */ }
func (c Code) String() string { return c.value }

// Aggregate root -- private fields, constructor with validation
type UOM struct { id uuid.UUID; code Code; name Name; /* ... */ }
func NewUOM(code, name string, category Category) (*UOM, error) { /* validate + construct */ }
func (u *UOM) ID() uuid.UUID { return u.id }
func (u *UOM) Update(code, name string, category Category) error { /* validate + mutate */ }
```

### Application Layer (`internal/application/<entity>/`)

Use cases implementing the Command/Query pattern. One handler per operation.

| File | Pattern |
|------|---------|
| `create_handler.go` | `CreateCommand` struct -> calls repo.Create -> returns `*Entity` |
| `get_handler.go` | `GetQuery` struct -> calls repo.GetByID -> returns `*Entity` |
| `list_handler.go` | `ListQuery` struct -> calls repo.List -> returns `[]*Entity` + total count |
| `update_handler.go` | `UpdateCommand` -> fetches entity -> calls entity.Update -> saves |
| `delete_handler.go` | `DeleteCommand` -> calls repo.Delete |
| `export_handler.go` | Generates Excel bytes |
| `import_handler.go` | Parses Excel bytes -> bulk creates |
| `template_handler.go` | Returns template Excel bytes |

Can only import from the domain layer.

### Infrastructure Layer (`internal/infrastructure/`)

Implements domain interfaces using external packages.

| Package | Purpose |
|---------|---------|
| `postgres/` | Repository implementations (pgx v5, parameterized queries) |
| `redis/` | Cache layer (nil-safe -- if cache pointer is nil, operations are no-ops) |
| `config/` | Viper YAML + env loading |
| `tracing/` | OpenTelemetry + OTLP exporter |
| `audit/` | Audit log recording |
| `jwt/` | JWT access/refresh tokens (IAM only) |
| `totp/` | TOTP 2FA (IAM only) |
| `password/` | bcrypt hashing (IAM only) |
| `email/` | SMTP service (IAM only) |
| `storage/` | S3/MinIO file storage (IAM only) |

Key pattern: infrastructure maps database errors to domain errors (e.g., PostgreSQL unique violation -> `domain.ErrAlreadyExists`).

### Delivery Layer (`internal/delivery/`)

Receives external requests, maps to application commands/queries, returns responses.

| Package | Purpose |
|---------|---------|
| `grpc/server.go` | gRPC server setup (keepalive, 10MB max message size) |
| `grpc/*_handler.go` | Maps proto requests to application commands, domain entities to proto responses |
| `grpc/interceptors.go` | Interceptor chain setup |
| `grpc/error_response.go` | `StructuredErrorInterceptor` -- catches all errors, wraps in BaseResponse |
| `grpc/auth_interceptor.go` | JWT validation, extracts user context (IAM) |
| `grpc/permission_interceptor.go` | RBAC permission checking per RPC (IAM) |
| `grpc/rate_limiter.go` | Token bucket rate limiting |
| `grpc/metrics.go` | Prometheus metrics interceptor |
| `httpdelivery/gateway.go` | gRPC-Gateway REST proxy + CORS + Swagger UI |

**Interceptor chain order**: StructuredError -> RequestID -> Timeout(30s) -> Logging -> RateLimit -> Auth(JWT) -> Permission(RBAC) -> Metrics -> Handler

---

## 6. Error Handling

### Pattern

1. **Domain**: define sentinel errors
   ```go
   var ErrNotFound = errors.New("uom not found")
   var ErrAlreadyExists = errors.New("uom code already exists")
   ```

2. **Infrastructure**: map external errors to domain errors
   ```go
   if pgErr.Code == "23505" { return uom.ErrAlreadyExists }
   ```

3. **Delivery**: `StructuredErrorInterceptor` catches all errors, maps domain errors to gRPC codes, wraps in `BaseResponse`

### Rules

- Always use `errors.Is()` / `errors.As()` -- **never** type assertions for error checking
- Always wrap errors with context: `fmt.Errorf("failed to create UOM: %w", err)`
- Never expose internal errors to clients (interceptor sanitizes to "internal server error")
- Every error must be handled -- never `result, _ := someFunc()`

---

## 7. Database Conventions

### Table Naming

| Prefix | Type | Example |
|--------|------|---------|
| `mst_` | Master data | `mst_uom`, `mst_user`, `mst_role`, `mst_menu` |
| `cst_` | Costing | (future) |
| `wfl_` | Workflow | (future) |
| (none) | Junction/relationship | `role_permissions`, `menu_permissions`, `user_roles` |

**Important**: Junction tables have NO prefix.

### Columns

- Always `snake_case`
- Primary key: `id UUID DEFAULT gen_random_uuid()`
- Audit trail: `created_at`, `created_by`, `updated_at`, `updated_by`
- Soft delete: `deleted_at TIMESTAMP WITH TIME ZONE`, `deleted_by VARCHAR`
- Use partial indexes on `deleted_at IS NULL` for active records
- Full-text search via `gin(to_tsvector(...))`

### Migrations

- Path: `migrations/postgres/`
- Naming: `NNNNNN_description.up.sql` / `NNNNNN_description.down.sql`
- Always idempotent: `IF NOT EXISTS` / `IF EXISTS`
- Never modify a merged migration -- create a new one
- Every up migration must have a corresponding down migration
- Tool: golang-migrate v4

### Seed Migration Guardrails

**Permission codes** must match the `chk_permission_code_format` CHECK constraint:
```
^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
```
Each dot-separated segment allows only lowercase letters and digits -- **no underscores, no hyphens**. Multi-word entities are concatenated: `employeelevel` not `employee_level`, `companymap` not `company_map`.

**Menu UUIDs** follow a deterministic convention per level. Before picking a UUID for a new menu seed, check what's already taken:
```bash
# Check existing level-2 menu UUIDs (section headers like "Master Data", "CMS"):
grep -h "00000000-0000-0000-0002-" migrations/postgres/*.up.sql | sort -u

# Check existing level-3 menu UUIDs (leaf pages like "Employee Level", "Roles"):
grep -h "00000000-0000-0000-0003-" migrations/postgres/*.up.sql | sort -u
```
Pick the next sequential number that is not already used. The `ON CONFLICT (menu_code) DO NOTHING` clause does NOT protect against `menu_id` collisions -- a new `menu_code` with a reused `menu_id` will fail with a primary key violation.

### Queries

- Always use parameterized queries (`$1`, `$2`) -- never string concatenation
- Always use transactions for multi-statement operations
- Always `defer rows.Close()` after `QueryContext`
- Use `context.Context` for all database operations

### Dirty Migration Fix

If a migration gets stuck as dirty:
```sql
UPDATE schema_migrations SET dirty = false WHERE version = <version>;
```

---

## 8. Testing Strategy

### Coverage Targets

| Layer | Minimum | Target |
|-------|---------|--------|
| Domain | 90% | 95% |
| Application | 80% | 90% |
| Infrastructure | 70% | 80% |
| Delivery | 60% | 70% |

### Patterns

- **Table-driven tests** with `testify` for all layers
- **Test naming**: `Test<Function>_<Scenario>_<ExpectedResult>`
- **Mocking**: use interfaces + `testify/mock` for dependencies
- **Integration tests**: gated by `INTEGRATION_TEST=true` env var, use real PostgreSQL
- **E2E tests**: gated by `E2E_TEST=true`, spin up full gRPC server

### Test File Location

| Test Type | Location |
|-----------|----------|
| Domain unit | `internal/domain/<entity>/*_test.go` |
| Application unit | `internal/application/<entity>/*_test.go` |
| Infrastructure integration | `internal/infrastructure/postgres/*_test.go` |
| Delivery unit | `internal/delivery/grpc/*_test.go` |
| E2E | `tests/e2e/*_test.go` |

---

## 9. Docker Compose (Local Development)

The root `docker-compose.yaml` starts all shared infrastructure:

| Service | Image | Host Port | Purpose |
|---------|-------|-----------|---------|
| `finance-postgres` | postgres:18-alpine | 5434 | Finance database |
| `iam-postgres` | postgres:18-alpine | 5435 | IAM database |
| `redis` | redis:7-alpine | 6379 | Shared cache + token blacklist |
| `mailpit` | axllent/mailpit | 1025 (SMTP), 8025 (UI) | Email testing |
| `jaeger` | jaegertracing/all-in-one:1.55 | 16686 (UI), 4317 (OTLP gRPC) | Distributed tracing |
| `minio` | minio/minio | 9000 (API), 9001 (Console) | S3-compatible storage |
| `minio-init` | minio/mc | -- | Creates `goapps-staging` bucket on startup |

Start everything: `docker compose up -d` from repo root.

Default credentials:
- Finance DB: `finance` / `finance123`
- IAM DB: `iam` / `iam123`
- MinIO: `minioadmin` / `minioadmin`
- Redis: no password

---

## 10. Lint Configuration

File: `.golangci.yml` (v2 format)

### Enabled Linters (27 total)

**Linters**: errcheck, govet, ineffassign, staticcheck, unused, bodyclose, dupl, errname, errorlint, exhaustive, gocognit, goconst, gocritic, gocyclo, gosec, misspell, nakedret, nestif, nilerr, nilnil, noctx, prealloc, predeclared, revive, unconvert, unparam, whitespace

**Formatters**: gofmt, goimports

### Key Thresholds

| Linter | Setting |
|--------|---------|
| gocyclo | max complexity 15 |
| gocognit | max complexity 20 |
| nestif | max nesting 4 |
| dupl | threshold 300 lines |
| goconst | min 3 occurrences, min length 3 |
| gosec | G104 excluded (unhandled errors on deferred Close) |
| exhaustive | `default-signifies-exhaustive: true` |

### Exclusions

- `gen/` directory is fully excluded
- Test files (`*_test.go`) are excluded from: dupl, gosec, goconst, gocognit

---

## 11. Key Dependencies

### Shared (both services)

| Package | Version | Purpose |
|---------|---------|---------|
| `google.golang.org/grpc` | v1.78 | gRPC framework |
| `grpc-ecosystem/grpc-gateway/v2` | v2.27 | REST gateway |
| `jackc/pgx/v5` | v5.8 | PostgreSQL driver (connection pooling) |
| `redis/go-redis/v9` | v9.17 | Redis client |
| `rs/zerolog` | v1.32 | Structured logging |
| `spf13/viper` | v1.18 | Configuration |
| `google/uuid` | v1.6 | UUID generation |
| `buf.build/go/protovalidate` | v1.1 | Proto request validation |
| `prometheus/client_golang` | v1.23 | Metrics |
| `go.opentelemetry.io/otel` | v1.39 | Distributed tracing |
| `stretchr/testify` | v1.11 | Testing assertions + mocks |
| `rs/cors` | v1.11 | CORS middleware |

### Finance-only

| Package | Purpose |
|---------|---------|
| `xuri/excelize/v2` | Excel import/export |

### IAM-only

| Package | Purpose |
|---------|---------|
| `golang-jwt/jwt/v5` | JWT token handling |
| `golang.org/x/crypto` | bcrypt password hashing |
| `minio/minio-go/v7` | S3-compatible file storage |

### Generated Code Module

Both services use `replace github.com/mutugading/goapps-backend/gen => ../../gen` in their `go.mod` to reference the shared generated proto code.

---

## 12. Configuration

Each service has a `config.yaml` at its root. Secrets come from environment variables.

### Finance (`services/finance/config.yaml`)

```yaml
app:    { name, version, env }
server: { grpc_port: 50051, http_port: 8080 }
database: { host, port, user, password, name, ssl_mode, pool settings }
redis:  { host, port, password, db: 0 }
auth_redis: { host, port, db: 1 }       # Shared token blacklist with IAM
jwt:    { access_token_secret, issuer }  # For validating IAM-issued tokens
cors:   { allowed_origins, max_age }
tracing: { enabled, endpoint }
rate_limit: { requests_per_second: 100, burst_size: 200 }
logging: { level, format }
```

### Environment Variables for Secrets

- `DATABASE_PASSWORD` -- database password
- `JWT_ACCESS_SECRET` / `JWT_REFRESH_SECRET` -- token signing keys
- `SEED_ADMIN_PASSWORD` -- IAM admin seed password (random if unset)
- Config values can be overridden via env: `DATABASE_HOST=prod-db ./bin/service`

---

## 13. Critical Rules

### Architecture

1. **Domain layer MUST NOT import external packages** -- standard library only
2. **Dependencies flow inward only**: Delivery -> Application -> Domain <- Infrastructure
3. **No circular imports** -- use interfaces to break dependency cycles
4. **Context is always the first parameter** in every function that does I/O

### Code Style

5. **No type stuttering**: use `uom.Code` not `uom.UOMCode`, `formula.Type` not `formula.FormulaType`, `formula.Param` not `formula.FormulaParam`. The `revive` linter enforces this -- if a type is inside package `foo`, its name must NOT start with `Foo`.
6. **Comments on exported types must end with a period.**
7. **Do not shadow builtins** (`min`, `max`, `len`, `cap`)
8. **Exhaustive switch** on all enum types
9. **Use pointers for protobuf structs**
10. Run `goimports -w .` before committing

### Error Handling

11. **Always use `errors.Is()` / `errors.As()`** -- never type assertions
12. **Always wrap errors** with context: `fmt.Errorf("doing X: %w", err)`
13. **Handle every error** -- never `result, _ := someFunc()`. This includes:
    - `tx.Rollback()` in deferred functions -- use `if rbErr := tx.Rollback(); rbErr != nil { ... }`
    - Value object constructors in repository mapping (`NewCode`, `NewType`, etc.) -- handle the error, don't ignore with `_`
    - The `errcheck` linter catches ALL of these

### Complexity & Safety

14. **Function cognitive complexity must stay under 20** (`gocognit`). Extract helper methods to reduce complexity -- e.g., `validateResultParam()`, `parseFormulaType()` instead of inline validation blocks.
15. **Max nesting depth is 4** (`nestif`). Flatten nested `if` blocks by extracting to separate functions.
16. **Safe integer conversions** (`gosec` G115). Never use bare `int32(someInt)` -- use a bounds-checking helper like `safeIntToInt32()` with `math.MaxInt32`/`math.MinInt32` clamping. **For int→int32 that is already bounds-checked earlier in the function**, add `//nolint:gosec // bounds checked above` on the same line as the cast.
17. **Repeated string literals ≥ 3 occurrences must be constants** (`goconst`, min length 3). When the same literal (e.g., `"UNSPECIFIED"`, `"create"`, `"update"`) appears 3+ times in a file, extract it to a `const` block. Common spots: enum `String()` methods, import error field names, duplicate action types.
18. **American English spelling in comments and identifiers** (`misspell`). Use `summarizes` (not `summarises`), `organization` (not `organisation`), `behavior` (not `behaviour`), `color` (not `colour`), `customize`, `authorize`, `initialize`. The linter runs on ALL comments including doc comments.

### Security

17. **Never commit secrets** -- use environment variables / config
18. **Always use parameterized queries** -- never string concatenation in SQL
19. **Never hardcode credentials** -- not even in dev configs (use config.yaml defaults)
20. **Super admin** bypasses RBAC via `IsSuperAdmin()` check in permission interceptor

### Database

21. **Master tables**: `mst_` prefix. **Junction tables**: no prefix
22. **Soft deletes**: `deleted_at` / `deleted_by` columns with partial indexes
23. **Migrations are immutable** once merged -- create new ones to fix issues

### Proto / Generated Code

24. **Never edit files in `gen/`** -- they are generated from `goapps-shared-proto`
25. **Never change proto field numbers**
26. **Never remove fields without `reserved`**
27. Regenerate: `cd ../goapps-shared-proto && ./scripts/gen-go.sh`

### Git

28. Branch naming: `feat/`, `fix/`, `docs/`, `refactor/`, `chore/` + description
29. Commit format: `type(scope): description` (e.g., `feat(finance): add currency CRUD`)

---

## 14. Lint Pre-Flight Checklist (MANDATORY before declaring work complete)

> CI runs golangci-lint v2.3.0. Local binary is v1.62.2, so `make lint` will not catch everything -- **manually review every new file** against this checklist. The most common CI failures are listed below with copy-pasteable fixes.

### Pattern 1 — int→int32 conversions (gosec G115)

```go
// ❌ FAILS CI
RowNumber: int32(row.RowNumber),
SortOrder: int32(fp.SortOrder()),

// ✅ Option A — use typed field upstream (preferred): declare RowNumber as int32 from the source.
// ✅ Option B — use bounds-checking helper:
func safeIntToInt32(v int) int32 {
    if v > math.MaxInt32 { return math.MaxInt32 }
    if v < math.MinInt32 { return math.MinInt32 }
    return int32(v) //nolint:gosec // bounds checked above
}

// ✅ Option C — if bounds were validated earlier in the same function, suppress with reason:
if grade < 0 || grade > 99 { return ErrInvalidGrade }
return int32(grade) //nolint:gosec // bounds checked above
```

Never suppress without a bounds check somewhere. The `//nolint` comment must cite the guarantee.

### Pattern 2 — repeated string literals (goconst, threshold 3)

If the SAME literal (length ≥ 3 chars) appears **3 or more times** in a single file, extract a const. Hotspots:
- Enum `String()` methods returning `"UNSPECIFIED"` for Unspecified + default case — duplicate across Type + Workflow + Status enums in the same file.
- Field names in `excel.ImportError{Field: "code", ...}` repeated for each error path.
- Action strings: `"create"`, `"update"`, `"skip"` in import handlers.

```go
// ❌ FAILS CI
case TypeUnspecified: return "UNSPECIFIED"
default: return "UNSPECIFIED"
// (and same string also used 2+ more times in another enum in the file)

// ✅ Fix
const unspecifiedLabel = "UNSPECIFIED"
case TypeUnspecified: return unspecifiedLabel
default: return unspecifiedLabel
```

### Pattern 3 — cognitive complexity ≤ 20 (gocognit)

Each `if`, `switch case`, `for`, `&&`, `||` adds to the score (nested blocks multiply). `Update` methods with 6+ optional field pointers almost always exceed 20. **Rule of thumb**: if a function has 5+ `if X != nil { validate; assign }` blocks, split each into `applyX()` helper method.

```go
// ❌ FAILS CI — complexity 21
func (e *Entity) Update(name *string, grade *int32, typ *Type, ...) error {
    if e.IsDeleted() { return ErrDeleted }
    if name != nil { if *name == "" { return ErrEmpty }; if len(*name) > 100 { return ErrTooLong }; e.name = *name }
    if grade != nil { if *grade < 0 { return ErrInvalidGrade }; e.grade = *grade }
    if typ != nil { if !typ.IsValid() { return ErrInvalidType }; e.typ = *typ }
    // ... 3 more blocks
}

// ✅ Fix — delegate to per-field helpers
func (e *Entity) Update(name *string, grade *int32, typ *Type, ...) error {
    if e.IsDeleted() { return ErrDeleted }
    if err := e.applyName(name); err != nil { return err }
    if err := e.applyGrade(grade); err != nil { return err }
    if err := e.applyType(typ); err != nil { return err }
    // ...
    return nil
}
func (e *Entity) applyName(name *string) error {
    if name == nil { return nil }
    if *name == "" { return ErrEmpty }
    if len(*name) > 100 { return ErrTooLong }
    e.name = *name
    return nil
}
```

### Pattern 4 — misspellings in comments (misspell, American English)

Accept: `summarizes`, `organization`, `behavior`, `color`, `customize`, `authorize`, `initialize`, `analyze`, `optimize`, `serialize`.
Reject: `summarises`, `organisation`, `behaviour`, `colour`, `customise`, `authorise`, `initialise`, `analyse`, `optimise`, `serialise`.

Runs on ALL comments (package, function, inline). Common offenders: doc comments on handlers (`// summarises outcomes`), inline comments (`// normalise input`).

### Pattern 5 — unhandled errors (errcheck)

```go
// ❌ FAILS CI
defer tx.Rollback()
code, _ := NewCode(row.Code)
_, _ = someCall()

// ✅ Fix
defer func() {
    if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
        log.Warn().Err(rbErr).Msg("rollback")
    }
}()
code, err := NewCode(row.Code)
if err != nil { return fmt.Errorf("parse code: %w", err) }
```

### Pattern 6 — nesting depth > 4 (nestif)

`if → for → if → if → if` = 5 levels = FAILS. Extract the inner block to a helper function.

### Pattern 7 — convertible struct literal (staticcheck S1016)

When two named struct types have **identical field names, types, and order**, copying them via field-by-field struct literal triggers `S1016: should convert ... instead of using struct literal`. Use a direct type conversion instead.

```go
// ❌ FAILS CI — staticcheck S1016
type UngroupedQuery struct {
    Search    string
    Scope     GroupingScope
    Page      int
    PageSize  int
    SortBy    string
    SortOrder string
}
type UngroupedItemsFilter struct {
    Search    string
    Scope     GroupingScope
    Page      int
    PageSize  int
    SortBy    string
    SortOrder string
}

func (h *Handler) Handle(_ context.Context, q UngroupedQuery) error {
    filter := UngroupedItemsFilter{
        Search:    q.Search,
        Scope:     q.Scope,
        Page:      q.Page,
        PageSize:  q.PageSize,
        SortBy:    q.SortBy,
        SortOrder: q.SortOrder,
    }
    // ...
}

// ✅ Fix — direct conversion
filter := UngroupedItemsFilter(q)
```

**Common spot**: Application-layer `XxxQuery` / `XxxCommand` structs that mirror an infrastructure-layer `XxxFilter`. Two options:

1. **Direct conversion** when the layers should stay decoupled but happen to have the same shape — staticcheck is happy, and adding a future field to one struct breaks the conversion at compile time so you can decide what to do.
2. **Type alias** (`type Filter = Query`) when you've decided the two are conceptually the same — eliminates the duplication entirely.

**When the structs DIVERGE** (different fields, different order, different types), keep the struct literal — staticcheck won't complain. If only the field order is different but contents match, **reorder the fields in one of the types** to match, then convert.

Watch this when:
- Adding a new field to a `Query`/`Command` struct that's also in `Filter` — keep the order matching.
- Refactoring a handler that just forwards a query to a repository.

### Pre-Commit Verification

Before declaring any backend task complete, run **all** of these. Finding even one issue blocks completion:

```bash
cd services/<service>
go build ./...                      # 0 compile errors
go vet ./...                        # 0 vet warnings
go test -race -count=1 ./...        # all tests pass
goimports -w .                      # formatting
# Visual review of new files: walk through Pattern 1-6 above manually,
# since local golangci-lint v1.62.2 will not catch v2-only rules.
```

If a handler or domain file adds new int32 casts, repeated strings, or enum `String()` methods, **always** inspect them against Patterns 1-3 before committing. If two struct types share identical shapes, inspect against Pattern 7.

---

## 15. Adding a New Feature (Checklist)

For a new CRUD entity in an existing service:

1. **Proto** (in `goapps-shared-proto/`): define messages, service RPCs, HTTP annotations, validation
2. **Generate**: `./scripts/gen-go.sh`
3. **Domain**: `entity.go`, `value_objects.go`, `repository.go`, `errors.go`
   - **No type stuttering**: if package is `formula`, use `Type` not `FormulaType`, `Param` not `FormulaParam`
4. **Migration**: `make migrate-create NAME=create_mst_xxx`
5. **Infrastructure**: `postgres/xxx_repository.go` implementing domain interface
   - **Handle ALL errors**: including `tx.Rollback()` in defers, and value object constructors in DTO mappers
   - **Transactions**: use `if rbErr := tx.Rollback(); rbErr != nil { err = fmt.Errorf(...) }` pattern
6. **Application**: one handler file per operation (create, get, list, update, delete)
   - **Keep cognitive complexity under 20**: extract validation helpers (e.g., `validateResultParam`, `parseFormulaType`)
   - **Keep nesting under 4 levels**: split deeply nested `if` blocks into helper methods
7. **Delivery**: `grpc/xxx_handler.go` mapping proto <-> application
   - **Safe int conversions**: never bare `int32(someInt)` -- use `safeIntToInt32()` with bounds checking
8. **Wire**: register in `cmd/server/main.go` (create repo, handlers, register gRPC service)
9. **Swagger**: `make proto-copy-swagger`
10. **Tests**: domain unit tests, application handler tests with mocked repo, integration tests
11. **Lint**: run `make lint` (or `golangci-lint run ./...`) and fix ALL issues before committing

### File Size Guidelines

| Type | Max Lines |
|------|-----------|
| Entity | 200 |
| Service/Handler | 300 |
| Repository | 400 |
| Test file | 500 |

Split by responsibility if exceeded.
