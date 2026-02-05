# Backend Development Rules

Guidelines and conventions for all developers working with `goapps-backend`.

---

## 📋 Table of Contents

1. [Golden Rules](#golden-rules)
2. [Clean Architecture](#clean-architecture)
3. [Naming Conventions](#naming-conventions)
4. [Code Organization](#code-organization)
5. [Error Handling](#error-handling)
6. [Logging Standards](#logging-standards)
7. [Testing Requirements](#testing-requirements)
8. [Proto/gRPC Conventions](#protogrpc-conventions)
9. [Database Guidelines](#database-guidelines)
10. [Git Workflow](#git-workflow)
11. [Performance Guidelines](#performance-guidelines)
12. [Security Best Practices](#security-best-practices)

---

## Golden Rules

> ⚠️ **Rules that MUST NOT be violated!**

### 1. Follow Clean Architecture

```
❌ WRONG - Domain depends on infrastructure
internal/domain/uom/entity.go importing "github.com/lib/pq"

✅ CORRECT - Domain has no external dependencies
internal/domain/uom/entity.go only imports standard library
```

### 2. No Circular Imports

```
❌ WRONG
package a imports package b
package b imports package a

✅ CORRECT
Use interfaces to break dependencies
```

### 3. Handle All Errors

```go
// ❌ WRONG
result, _ := someFunction()

// ✅ CORRECT
result, err := someFunction()
if err != nil {
    return fmt.Errorf("someFunction failed: %w", err)
}
```

### 4. Use Context

```go
// ❌ WRONG - No context
func (r *Repository) GetByID(id string) (*Entity, error)

// ✅ CORRECT - Context first
func (r *Repository) GetByID(ctx context.Context, id string) (*Entity, error)
```

### 5. Never Commit Secrets

```go
// ❌ WRONG - Hardcoded credentials
password := "mysecretpassword"

// ✅ CORRECT - Use environment/config
password := cfg.Database.Password
```

---

## Clean Architecture

### Layer Responsibilities

| Layer | Responsibility | Can Import |
|-------|----------------|------------|
| **Domain** | Entities, value objects, repository interfaces, domain errors | Standard library only |
| **Application** | Use cases, business logic, DTOs, mappers | Domain |
| **Infrastructure** | Database, cache, config, external services | Domain, Application, External packages |
| **Delivery** | gRPC handlers, HTTP handlers, middleware | Domain, Application, Infrastructure |

### Domain Layer

```go
// internal/domain/uom/entity.go
package uom

import (
    "time"
    // NO external imports!
)

type UOM struct {
    ID        string
    Code      string
    Name      string
    IsActive  bool
    CreatedAt time.Time
    UpdatedAt time.Time
}

// Domain methods operating on entity
func (u *UOM) Activate() {
    u.IsActive = true
}
```

### Repository Interface (Domain)

```go
// internal/domain/uom/repository.go
package uom

import "context"

// Repository defines the contract - implementation is in infrastructure
type Repository interface {
    Create(ctx context.Context, uom *UOM) error
    GetByID(ctx context.Context, id string) (*UOM, error)
    Update(ctx context.Context, uom *UOM) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter Filter) ([]UOM, int, error)
}
```

### Application Layer

```go
// internal/application/uom/service.go
package uom

import (
    "context"
    
    "github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

type Service struct {
    repo  uom.Repository  // Depends on interface, not implementation
    cache Cache           // Optional cache interface
}

func (s *Service) CreateUOM(ctx context.Context, dto CreateUOMDTO) (*UOMResponseDTO, error) {
    // Business logic here
    entity := mapToEntity(dto)
    
    if err := s.repo.Create(ctx, entity); err != nil {
        return nil, err
    }
    
    return mapToDTO(entity), nil
}
```

### Infrastructure Layer

```go
// internal/infrastructure/postgres/uom_repository.go
package postgres

import (
    "context"
    
    "github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

// Implements uom.Repository interface
type UOMRepository struct {
    db *DB
}

func (r *UOMRepository) Create(ctx context.Context, entity *uom.UOM) error {
    query := `INSERT INTO uom (id, code, name) VALUES ($1, $2, $3)`
    _, err := r.db.ExecContext(ctx, query, entity.ID, entity.Code, entity.Name)
    return err
}
```

---

## Naming Conventions

### Packages

| Type | Convention | Example |
|------|------------|---------|
| Domain entity | Singular noun | `uom`, `user`, `order` |
| Infrastructure | Technology name | `postgres`, `redis`, `grpc` |
| Utility | Descriptive | `logger`, `validator`, `converter` |

### Files

| Type | Convention | Example |
|------|------------|---------|
| Entity | `entity.go` | `entity.go` |
| Value object | `value_object.go` | `value_object.go` |
| Repository interface | `repository.go` | `repository.go` |
| Repository impl | `<entity>_repository.go` | `uom_repository.go` |
| Service | `service.go` | `service.go` |
| Handler | `<entity>_handler.go` | `uom_handler.go` |
| Tests | `*_test.go` | `entity_test.go` |

### Functions and Methods

| Type | Convention | Example |
|------|------------|---------|
| Constructor | `New<Type>` | `NewUOMRepository(db *DB)` |
| Getter | `Get<Property>` or just property | `GetName()` or `Name()` |
| Setter | `Set<Property>` | `SetName(name string)` |
| Boolean | `Is<Condition>` or `Has<Thing>` | `IsActive()`, `HasPermission()` |
| Collection | Plural | `ListUOMs()`, `GetUsers()` |
| CRUD | `Create`, `Get`, `Update`, `Delete` | `CreateUOM()` |

### Variables

```go
// ✅ Good - Descriptive
var userRepository Repository
var connectionTimeout time.Duration
var maxRetries int

// ❌ Bad - Abbreviated/unclear
var ur Repository
var ct time.Duration
var mr int

// ✅ Good - Short names for short scopes
for i, item := range items {
    // ...
}

// ✅ Good - Context is always ctx
func (s *Service) GetByID(ctx context.Context, id string) error
```

### Constants

```go
// ✅ Good - Grouped and prefixed
const (
    DefaultTimeout       = 30 * time.Second
    DefaultMaxRetries    = 3
    DefaultPoolSize      = 25
)

// ✅ Good - Enum pattern
type Status string

const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive"
    StatusDeleted  Status = "deleted"
)
```

---

## Code Organization

### Service Directory Structure

```
services/<service-name>/
├── cmd/
│   └── server/
│       └── main.go           # Entry point only, minimal code
├── internal/
│   ├── domain/
│   │   └── <entity>/
│   │       ├── entity.go
│   │       ├── entity_test.go
│   │       ├── value_object.go
│   │       ├── repository.go
│   │       └── errors.go
│   ├── application/
│   │   └── <entity>/
│   │       ├── service.go
│   │       ├── service_test.go
│   │       ├── dto.go
│   │       └── mapper.go
│   ├── infrastructure/
│   │   ├── config/
│   │   ├── postgres/
│   │   ├── redis/
│   │   └── tracing/
│   └── delivery/
│       ├── grpc/
│       └── httpdelivery/
├── pkg/                      # Shared utilities (can be imported externally)
├── migrations/
├── tests/
├── Dockerfile
├── Makefile
└── go.mod
```

### File Size Guidelines

| Type | Recommended Max Lines |
|------|----------------------|
| Entity | 200 |
| Service | 300 |
| Repository | 400 |
| Handler | 300 |
| Test file | 500 |

If a file exceeds these limits, consider splitting by responsibility.

---

## Error Handling

### Domain Errors

```go
// internal/domain/uom/errors.go
package uom

import "errors"

var (
    ErrNotFound       = errors.New("uom not found")
    ErrAlreadyExists  = errors.New("uom already exists")
    ErrInvalidCode    = errors.New("invalid uom code")
    ErrInvalidName    = errors.New("invalid uom name")
)
```

### Error Wrapping

```go
// ✅ Good - Wrap with context
if err := r.db.ExecContext(ctx, query, args...); err != nil {
    return fmt.Errorf("failed to create UOM: %w", err)
}

// ✅ Good - Check for specific errors
if errors.Is(err, uom.ErrNotFound) {
    return status.Error(codes.NotFound, "UOM not found")
}
```

### gRPC Error Mapping

```go
// internal/delivery/grpc/errors.go
package grpc

import (
    "errors"
    
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
    
    "github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

func mapError(err error) error {
    switch {
    case errors.Is(err, uom.ErrNotFound):
        return status.Error(codes.NotFound, err.Error())
    case errors.Is(err, uom.ErrAlreadyExists):
        return status.Error(codes.AlreadyExists, err.Error())
    case errors.Is(err, uom.ErrInvalidCode), errors.Is(err, uom.ErrInvalidName):
        return status.Error(codes.InvalidArgument, err.Error())
    default:
        return status.Error(codes.Internal, "internal server error")
    }
}
```

---

## Logging Standards

### Log Levels

| Level | Usage |
|-------|-------|
| `Debug` | Detailed debugging info (development only) |
| `Info` | Normal operational messages |
| `Warn` | Non-critical issues, degraded functionality |
| `Error` | Errors that need attention |
| `Fatal` | Unrecoverable errors, application exit |

### Structured Logging

```go
import "github.com/rs/zerolog/log"

// ✅ Good - Structured with context
log.Info().
    Str("service", "finance").
    Str("action", "create_uom").
    Str("uom_id", uom.ID).
    Str("uom_code", uom.Code).
    Msg("UOM created successfully")

// ✅ Good - Error with details
log.Error().
    Err(err).
    Str("uom_id", id).
    Msg("Failed to get UOM")

// ❌ Bad - Unstructured
log.Info().Msgf("Created UOM: %s %s", uom.ID, uom.Code)
```

### What to Log

| Event | Level | Fields |
|-------|-------|--------|
| Request received | Debug | method, path, trace_id |
| Request completed | Info | method, path, status, duration |
| Database query | Debug | query, args, duration |
| Cache hit/miss | Debug | key, hit/miss |
| External call | Info | service, method, duration |
| Error occurred | Error | error, stack, context |
| Service started | Info | version, env, config |
| Service stopped | Info | reason, uptime |

---

## Testing Requirements

### Test Coverage

| Type | Minimum Coverage | Target |
|------|-----------------|--------|
| Domain | 90% | 95% |
| Application | 80% | 90% |
| Infrastructure | 70% | 80% |
| Delivery | 60% | 70% |

### Test Naming

```go
// Format: Test<Function>_<Scenario>_<ExpectedResult>

func TestCreateUOM_ValidInput_ReturnsUOM(t *testing.T)
func TestCreateUOM_DuplicateCode_ReturnsError(t *testing.T)
func TestCreateUOM_EmptyName_ReturnsValidationError(t *testing.T)
```

### Table-Driven Tests

```go
func TestUOM_Validate(t *testing.T) {
    tests := []struct {
        name    string
        uom     UOM
        wantErr bool
    }{
        {
            name:    "valid uom",
            uom:     UOM{Code: "KG", Name: "Kilogram"},
            wantErr: false,
        },
        {
            name:    "empty code",
            uom:     UOM{Code: "", Name: "Kilogram"},
            wantErr: true,
        },
        {
            name:    "empty name",
            uom:     UOM{Code: "KG", Name: ""},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.uom.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Mocking

Use interfaces for dependencies to enable mocking:

```go
// Mock repository
type mockRepository struct {
    mock.Mock
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*uom.UOM, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*uom.UOM), args.Error(1)
}
```

---

## Proto/gRPC Conventions

### Message Naming

```protobuf
// Request: <Method>Request
message CreateUOMRequest {
  string code = 1;
  string name = 2;
}

// Response: <Method>Response
message CreateUOMResponse {
  UOM uom = 1;
}

// Entity: Singular noun
message UOM {
  string id = 1;
  string code = 2;
  string name = 3;
}
```

### Field Numbering

```protobuf
message UOM {
  // Core fields: 1-15 (most frequently used)
  string id = 1;
  string code = 2;
  string name = 3;
  bool is_active = 4;
  
  // Timestamps: 16-20
  google.protobuf.Timestamp created_at = 16;
  google.protobuf.Timestamp updated_at = 17;
  
  // Relations: 21+
  string category_id = 21;
  repeated string tag_ids = 22;
}
```

### Validation

Use buf validate:

```protobuf
import "buf/validate/validate.proto";

message CreateUOMRequest {
  string code = 1 [(buf.validate.field).string = {
    min_len: 1,
    max_len: 10,
    pattern: "^[A-Z0-9]+$"
  }];
  
  string name = 2 [(buf.validate.field).string = {
    min_len: 1,
    max_len: 100
  }];
}
```

---

## Database Guidelines

### Migration Rules

1. **Always use migrations** - Never modify schema manually
2. **Migrations are immutable** - Once merged, never modify
3. **Include rollback** - Every up migration needs down migration
4. **Descriptive names** - `000001_create_uom_table.up.sql`

### Migration Format

```sql
-- 000001_create_uom_table.up.sql
CREATE TABLE IF NOT EXISTS uom (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(10) NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_uom_code ON uom(code);
CREATE INDEX idx_uom_is_active ON uom(is_active);
```

### Query Best Practices

```go
// ✅ Good - Use prepared statements
query := `SELECT id, code, name FROM uom WHERE id = $1`
row := r.db.QueryRowContext(ctx, query, id)

// ✅ Good - Use transactions for multiple operations
tx, err := r.db.BeginTx(ctx, nil)
if err != nil {
    return err
}
defer tx.Rollback()

// ... operations ...

return tx.Commit()

// ❌ Bad - String concatenation
query := "SELECT * FROM uom WHERE id = '" + id + "'"
```

---

## Git Workflow

### Branch Naming

| Type | Pattern | Example |
|------|---------|---------|
| Feature | `feat/<service>/<description>` | `feat/finance/add-uom-export` |
| Bug fix | `fix/<service>/<description>` | `fix/finance/uom-validation` |
| Hotfix | `hotfix/<description>` | `hotfix/database-connection` |
| Refactor | `refactor/<description>` | `refactor/uom-repository` |
| Docs | `docs/<description>` | `docs/api-documentation` |

### Commit Messages

Format: `<type>(<scope>): <description>`

```bash
# Features
feat(uom): add bulk import from Excel
feat(uom): implement soft delete

# Bug fixes
fix(uom): handle duplicate code error
fix(redis): fix connection timeout

# Refactoring
refactor(uom): extract validation logic
refactor(postgres): simplify query builder

# Documentation
docs(readme): update quick start guide
docs(api): add OpenAPI examples

# Chores
chore(deps): upgrade grpc to 1.78.0
chore(ci): add integration test job
```

---

## Performance Guidelines

### Resource Management

```go
// ✅ Good - Use connection pooling
db, _ := sql.Open("postgres", dsn)
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)

// ✅ Good - Close resources
rows, err := db.QueryContext(ctx, query)
if err != nil {
    return err
}
defer rows.Close()
```

### Caching Strategy

```go
// ✅ Good - Cache with TTL
func (c *Cache) GetUOM(ctx context.Context, id string) (*uom.UOM, error) {
    key := "uom:" + id
    
    // Try cache first
    cached, err := c.redis.Get(ctx, key).Result()
    if err == nil {
        var result uom.UOM
        json.Unmarshal([]byte(cached), &result)
        return &result, nil
    }
    
    // Fallback to database
    result, err := c.repo.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }
    
    // Cache for future requests
    data, _ := json.Marshal(result)
    c.redis.Set(ctx, key, data, 5*time.Minute)
    
    return result, nil
}
```

### Context Timeouts

```go
// ✅ Good - Set appropriate timeouts
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

result, err := r.db.QueryContext(ctx, query, args...)
```

---

## Security Best Practices

### Input Validation

```go
// ✅ Good - Validate at boundary
func (h *Handler) CreateUOM(ctx context.Context, req *pb.CreateUOMRequest) (*pb.CreateUOMResponse, error) {
    // Validate request
    if err := req.Validate(); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    
    // Sanitize input
    code := strings.TrimSpace(strings.ToUpper(req.Code))
    name := strings.TrimSpace(req.Name)
    
    // ...
}
```

### Secrets Management

```go
// ✅ Good - Use environment variables
type DatabaseConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Password string `mapstructure:"password"` // From env var
    Name     string `mapstructure:"name"`
}

// config.yaml - No secrets!
database:
  host: ${DATABASE_HOST:localhost}
  port: ${DATABASE_PORT:5432}
  user: ${DATABASE_USER:postgres}
  password: ${DATABASE_PASSWORD}  # Required env var
  name: ${DATABASE_NAME:app}
```

### SQL Injection Prevention

```go
// ❌ NEVER do this
query := fmt.Sprintf("SELECT * FROM users WHERE name = '%s'", name)

// ✅ Always use parameterized queries
query := "SELECT * FROM users WHERE name = $1"
row := db.QueryRowContext(ctx, query, name)
```

---

## Contact

- **Team**: GoApps Backend
- **Slack**: #goapps-backend
- **Escalation**: Team Lead → Tech Lead → CTO
