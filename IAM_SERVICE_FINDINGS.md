# IAM Service — Deep Research & Findings Report

**Repo:** `goapps-backend`  
**Date:** 2026-02-27  
**Focus:** `services/iam/` — Single Sign-On IAM Microservice (DDD + gRPC)  
**Analisis oleh:** Kilo Code Deep Research

---

## Daftar Isi

1. [Struktur Direktori Lengkap](#1-struktur-direktori-lengkap)
2. [Arsitektur & Pola DDD](#2-arsitektur--pola-ddd)
3. [Domain Layer — Detail Lengkap](#3-domain-layer--detail-lengkap)
4. [Application Layer — Detail Lengkap](#4-application-layer--detail-lengkap)
5. [Infrastructure Layer — Detail Lengkap](#5-infrastructure-layer--detail-lengkap)
6. [Delivery Layer — Detail Lengkap](#6-delivery-layer--detail-lengkap)
7. [Database Schema — Detail Lengkap](#7-database-schema--detail-lengkap)
8. [Alur Autentikasi & Otorisasi](#8-alur-autentikasi--otorisasi)
9. [gRPC Services & REST Gateway](#9-grpc-services--rest-gateway)
10. [Keamanan & Kriptografi](#10-keamanan--kriptografi)
11. [Observability (Logging, Metrics, Tracing)](#11-observability-logging-metrics-tracing)
12. [Testing Coverage](#12-testing-coverage)
13. [Temuan: Bug & Masalah Kritis](#13-temuan-bug--masalah-kritis)
14. [Temuan: Gap Fungsionalitas](#14-temuan-gap-fungsionalitas)
15. [Temuan: Inkonsistensi Pola](#15-temuan-inkonsistensi-pola)
16. [Temuan: Technical Debt](#16-temuan-technical-debt)
17. [Evaluasi SSO Cross-Service](#17-evaluasi-sso-cross-service)
18. [Ringkasan Temuan](#18-ringkasan-temuan)

---

## 1. Struktur Direktori Lengkap

```
goapps-backend/
├── .github/
│   ├── workflows/
│   │   ├── finance-service.yml       # CI/CD Finance (IAM belum punya CI/CD!)
│   │   └── release-please.yml        # Release automation
│   └── ISSUE_TEMPLATE/
├── .golangci.yml                      # golangci-lint v2.3.0 config
├── docker-compose.yaml                # Shared infra: iam-postgres:5435,
│                                      # finance-postgres:5434, redis:6379,
│                                      # mailpit, jaeger
├── gen/                               # Generated protobuf code (module terpisah)
│   ├── common/v1/common.pb.go
│   ├── finance/v1/uom*.pb.go
│   ├── iam/v1/
│   │   ├── audit*.pb.go / *_grpc.pb.go / *.pb.gw.go
│   │   ├── auth*.pb.go / *_grpc.pb.go / *.pb.gw.go
│   │   ├── menu*.pb.go / ...
│   │   ├── organization*.pb.go / ...
│   │   ├── role*.pb.go / ...
│   │   ├── session*.pb.go / ...
│   │   └── user*.pb.go / ...
│   ├── openapi/                       # OpenAPI/Swagger specs
│   ├── go.mod
│   └── go.sum
├── Makefile                           # Root Makefile
├── README.md
├── RULES.md                           # Aturan coding & arsitektur
└── services/
    ├── finance/                       # Finance Microservice (port 50051/8080)
    └── iam/                           # IAM Microservice (port 50052/8081)
        ├── .env.example
        ├── CHANGELOG.md
        ├── DEVELOPMENT.md
        ├── Dockerfile
        ├── Makefile
        ├── cmd/server/main.go          # Entry point
        ├── config.yaml
        ├── deployments/
        │   └── docker-compose.yaml
        ├── go.mod / go.sum
        ├── internal/
        │   ├── application/
        │   │   ├── audit/
        │   │   │   ├── get_handler.go
        │   │   │   ├── list_handler.go
        │   │   │   └── summary_handler.go
        │   │   ├── auth/
        │   │   │   └── service.go     # Login, Logout, Refresh, ForgotPwd, OTP, 2FA
        │   │   ├── menu/
        │   │   │   └── service.go
        │   │   ├── organization/
        │   │   │   └── service.go
        │   │   ├── permission/
        │   │   │   ├── create_handler.go
        │   │   │   ├── delete_handler.go
        │   │   │   ├── get_handler.go
        │   │   │   ├── list_handler.go
        │   │   │   └── update_handler.go
        │   │   ├── role/
        │   │   │   ├── assign_permissions_handler.go
        │   │   │   ├── create_handler.go
        │   │   │   ├── delete_handler.go
        │   │   │   ├── get_handler.go
        │   │   │   ├── list_handler.go
        │   │   │   └── update_handler.go
        │   │   ├── session/
        │   │   │   ├── list_handler.go
        │   │   │   └── revoke_handler.go
        │   │   └── user/
        │   │       ├── create_handler.go
        │   │       ├── delete_handler.go
        │   │       ├── get_handler.go
        │   │       ├── list_handler.go
        │   │       ├── roles_handler.go
        │   │       ├── permissions_handler.go
        │   │       └── update_handler.go
        │   ├── delivery/
        │   │   ├── grpc/
        │   │   │   ├── audit_handler.go
        │   │   │   ├── auth_handler.go
        │   │   │   ├── auth_interceptor.go
        │   │   │   ├── auth_interceptor_test.go
        │   │   │   ├── company_handler.go
        │   │   │   ├── department_handler.go
        │   │   │   ├── division_handler.go
        │   │   │   ├── error_response.go
        │   │   │   ├── error_response_test.go
        │   │   │   ├── helpers.go
        │   │   │   ├── interceptors.go
        │   │   │   ├── menu_handler.go
        │   │   │   ├── metrics.go
        │   │   │   ├── permission_handler.go
        │   │   │   ├── permission_interceptor.go
        │   │   │   ├── permission_interceptor_test.go
        │   │   │   ├── rate_limiter.go
        │   │   │   ├── role_handler.go
        │   │   │   ├── section_handler.go
        │   │   │   ├── server.go
        │   │   │   ├── session_handler.go
        │   │   │   ├── user_handler.go
        │   │   │   └── validation_helper.go
        │   │   └── httpdelivery/
        │   │       ├── gateway.go
        │   │       └── swagger.json
        │   ├── domain/
        │   │   ├── audit/
        │   │   │   ├── entity.go
        │   │   │   └── repository.go
        │   │   ├── auth/
        │   │   │   └── service.go     # Interface: AuthService
        │   │   ├── menu/
        │   │   │   ├── entity.go
        │   │   │   ├── entity_test.go
        │   │   │   └── repository.go
        │   │   ├── organization/
        │   │   │   ├── entity.go
        │   │   │   ├── entity_test.go
        │   │   │   └── repository.go
        │   │   ├── role/
        │   │   │   ├── entity.go
        │   │   │   ├── entity_test.go
        │   │   │   └── repository.go
        │   │   ├── session/
        │   │   │   ├── entity.go
        │   │   │   └── repository.go
        │   │   ├── shared/
        │   │   │   ├── audit.go       # AuditInfo value object
        │   │   │   └── errors.go      # Shared domain errors
        │   │   └── user/
        │   │       ├── detail.go
        │   │       ├── entity.go
        │   │       ├── entity_test.go
        │   │       └── repository.go
        │   └── infrastructure/
        │       ├── config/config.go
        │       ├── email/service.go
        │       ├── jwt/service.go
        │       ├── password/service.go
        │       ├── password/service_test.go
        │       ├── postgres/
        │       │   ├── audit_repository.go
        │       │   ├── connection.go
        │       │   ├── menu_repository.go
        │       │   ├── organization_repository.go
        │       │   ├── permission_repository.go
        │       │   ├── role_repository.go
        │       │   ├── role_repository_test.go
        │       │   ├── session_repository.go
        │       │   ├── test_helpers_test.go
        │       │   ├── user_permission_repository.go
        │       │   ├── user_repository.go
        │       │   ├── user_repository_test.go
        │       │   └── user_role_repository.go
        │       ├── redis/cache.go
        │       ├── totp/service.go
        │       └── tracing/tracing.go
        ├── migrations/postgres/
        │   ├── 000001_create_organization_tables.up/down.sql
        │   ├── 000002_create_user_tables.up/down.sql
        │   ├── 000003_create_auth_tables.up/down.sql
        │   ├── 000004_create_rbac_tables.up/down.sql
        │   ├── 000005_create_menu_tables.up/down.sql
        │   ├── 000006_create_audit_tables.up/down.sql
        │   └── 000007_create_recovery_codes_table.up/down.sql
        ├── pkg/
        │   ├── logger/logger.go
        │   └── safeconv/safeconv.go
        ├── seeds/main.go
        └── tests/e2e/
            ├── auth_test.go
            ├── helpers_test.go
            ├── role_test.go
            └── user_test.go
```

---

## 2. Arsitektur & Pola DDD

### Clean Architecture (4 Lapisan)

Proyek mengikuti **Clean Architecture** secara ketat sebagaimana didokumentasikan dalam `RULES.md`:

```
┌──────────────────────────────────────────────────────────────────┐
│  DELIVERY (grpc/, httpdelivery/)                                  │
│  → Translate proto ↔ domain, orchestrate application handlers    │
├──────────────────────────────────────────────────────────────────┤
│  APPLICATION (application/)                                       │
│  → Use cases: CreateUser, Login, AssignPermissions, etc.          │
│  → Command/Query handler pattern (CQRS-lite)                      │
├──────────────────────────────────────────────────────────────────┤
│  DOMAIN (domain/)                                                 │
│  → Entities, Value Objects, Repository Interfaces, Domain Errors  │
│  → Zero external dependencies (stdlib only)                       │
├──────────────────────────────────────────────────────────────────┤
│  INFRASTRUCTURE (infrastructure/)                                 │
│  → PostgreSQL repos, Redis caches, JWT, TOTP, Email, Config      │
│  → Implements domain interfaces                                   │
└──────────────────────────────────────────────────────────────────┘
```

**Aturan Dependency:**
- Domain ← Application ← Infrastructure ← Delivery
- Domain tidak boleh import layer manapun
- Application hanya boleh import Domain interfaces (bukan implementasi)
- Dependency Inversion Principle diterapkan penuh

### Pola DDD yang Diimplementasikan

| Pola | Status | Detail |
|------|--------|--------|
| **Aggregate Root** | ✅ Implemented | `User`, `Role`, `Permission`, `Menu`, `Company`, etc. |
| **Value Object** | ✅ Implemented | `AuditInfo`, `Code` (UOM), `Category` (UOM) |
| **Repository Pattern** | ✅ Implemented | Interface di domain, implementasi di infra |
| **Domain Events** | ❌ Not Implemented | Tidak ada event bus atau domain events |
| **Factory Method** | ✅ Partial | `New*()` untuk create, `Reconstruct*()` untuk hydration dari DB |
| **Invariant Enforcement** | ✅ Implemented | Validasi di konstruktor domain |
| **Ubiquitous Language** | ✅ Implemented | Konsisten di kode, DB, API |
| **Bounded Context** | ✅ Implemented | IAM dan Finance sebagai bounded context terpisah |
| **CQRS-lite** | ✅ Partial | Handler terpisah per use-case, tanpa event sourcing |
| **Shared Kernel** | ✅ Implemented | `shared/audit.go`, `shared/errors.go` |
| **Saga/Process Manager** | ❌ Not Implemented | Tidak ada distributed transactions |

---

## 3. Domain Layer — Detail Lengkap

### 3.1 User Domain (`domain/user/`)

**Entity: `User`** (aggregate root)

```
Fields:
  - id UUID
  - username string (3-50 chars, ^[a-zA-Z][a-zA-Z0-9_]*$)
  - email string (valid email format)
  - passwordHash string
  - isActive bool
  - isLocked bool
  - failedLoginAttempts int
  - lockedUntil *time.Time
  - twoFactorEnabled bool
  - twoFactorSecret string       ⚠ Plaintext! (harusnya encrypted)
  - lastLoginAt *time.Time
  - lastLoginIP string
  - passwordChangedAt *time.Time
  - AuditInfo (embedded)

Domain Methods:
  - CanLogin() error              → checks isActive, isLocked, lockExpiry
  - RecordLoginSuccess(ip)        → clears failedAttempts, sets lastLoginAt
  - RecordLoginFailure(maxAttempts, lockDuration) → increments counter, locks if needed
  - Enable2FA(secret)             → validates state, sets secret
  - Disable2FA()                  → clears secret
  - UpdatePassword(hash)          → updates hash + passwordChangedAt
  - Activate() / Deactivate()
```

**Entity: `UserDetail`** — profile data terlampir ke User

```
Fields:
  - id UUID
  - userID UUID (FK)
  - sectionID *UUID (nullable FK → mst_section)
  - employeeCode string (unique)
  - fullName, firstName, lastName string
  - phone string
  - profilePictureURL string
  - position string
  - dateOfBirth *time.Time
  - address string
  - extraData map[string]interface{} (JSONB)
```

**Repository Interface: `user.Repository`**

```go
type Repository interface {
    Create(ctx, user *User) error
    GetByID(ctx, id UUID) (*User, error)
    GetByUsername(ctx, username string) (*User, error)
    GetByEmail(ctx, email string) (*User, error)
    Update(ctx, user *User) error
    SoftDelete(ctx, id UUID, deletedBy UUID) error
    List(ctx, filter Filter) ([]*User, int64, error)
    ExistsUsername(ctx, username string) (bool, error)
    ExistsEmail(ctx, email string) (bool, error)
    GetDetailByUserID(ctx, userID UUID) (*UserDetail, error)
    CreateDetail(ctx, detail *UserDetail) error
    UpdateDetail(ctx, detail *UserDetail) error
    ListWithDetails(ctx, filter Filter) ([]*UserWithDetail, int64, error)
    UpdateFailedLoginAttempts(ctx, user *User) error
    UpdateLockStatus(ctx, user *User) error
    UpdateLastLogin(ctx, user *User) error
    GetRolesByUserID(ctx, userID UUID) ([]*role.Role, error)     // cross-domain ref
    GetPermissionsByUserID(ctx, userID UUID) ([]*role.Permission, error)
}
```

---

### 3.2 Role Domain (`domain/role/`)

**Entity: `Role`** (aggregate root)

```
Fields:
  - id UUID
  - code string (^[A-Z][A-Z0-9_]*$)
  - name string (2-100 chars)
  - description string
  - isSystem bool         → system roles cannot be deleted (ErrSystemRoleDelete)
  - isActive bool
  - AuditInfo (embedded)

Domain Methods:
  - CanDelete() error     → checks isSystem flag
  - Activate() / Deactivate()
```

**Entity: `Permission`**

```
Fields:
  - id UUID
  - code string (^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$)
               e.g. "iam.user.account.create", "finance.master.uom.view"
  - name string
  - serviceName string    → for cross-service permission grouping
  - moduleName string
  - actionType string     (CHECK: view/create/update/delete/export/import)
  - isActive bool

Note: Tidak memiliki domain methods selain konstruktor — anemic entity
```

**Repository Interfaces (Role Domain, 4 interfaces):**

```go
type Repository interface                 // role CRUD + list
type PermissionRepository interface       // permission CRUD + list + getByService
type UserRoleRepository interface         // assign/remove/get user roles
type UserPermissionRepository interface   // assign/remove/get direct user permissions
```

---

### 3.3 Session Domain (`domain/session/`)

**Entity: `Session`**

```
Fields:
  - id UUID
  - userID UUID
  - refreshTokenHash string  (SHA256 dari token)
  - deviceInfo string
  - ipAddress string
  - serviceName string       → service mana yang issue token
  - expiresAt time.Time
  - revokedAt *time.Time
  - createdAt time.Time

Domain Methods:
  - IsActive() bool          → revokedAt IS NULL AND expiresAt > now
  - IsExpired() bool
  - Revoke()                 → sets revokedAt = now
  - ValidateToken(token) error → hash + compare
```

**Repository Interfaces (2 interfaces):**

```go
type Repository interface {
    Create(ctx, session *Session) error
    GetByID(ctx, id UUID) (*Session, error)
    GetByRefreshToken(ctx, tokenHash string) (*Session, error)
    GetByTokenID(ctx, tokenID string) (*Session, error)     // by JTI
    RevokeByID(ctx, id UUID) error
    RevokeByTokenID(ctx, tokenID string) error
    RevokeAllByUserID(ctx, userID UUID) error               // force logout all
    ListActiveByUserID(ctx, userID UUID) ([]*Session, error)
    CleanupExpired(ctx) error                               // ⚠ Not called anywhere (GC gap)
}

type CacheRepository interface {
    StoreSession(ctx, session *Session, ttl time.Duration) error
    GetSession(ctx, id string) (*Session, error)
    DeleteSession(ctx, id string) error
    IsTokenBlacklisted(ctx, tokenID string) (bool, error)
    BlacklistToken(ctx, tokenID string, ttl time.Duration) error
}
```

---

### 3.4 Organization Domain (`domain/organization/`)

**4 entitas hierarkis:**

```
Company
  └── Division (FK: company_id)
        └── Department (FK: division_id)
              └── Section (FK: department_id)

Setiap entitas:
  - id UUID
  - code string (^[A-Z][A-Z0-9_]*$)
  - name string
  - description string
  - isActive bool
  - parentID UUID (FK ke parent)
  - AuditInfo (embedded)
```

**Repository Interface (1 interface mengelola semua 4 entitas):**

```go
type Repository interface {
    // Company
    CreateCompany(ctx, company *Company) error
    GetCompanyByID(ctx, id UUID) (*Company, error)
    UpdateCompany(ctx, company *Company) error
    SoftDeleteCompany(ctx, id UUID, deletedBy UUID) error
    ListCompanies(ctx, filter OrgFilter) ([]*Company, int64, error)
    ExistsCompanyCode(ctx, code string) (bool, error)

    // Division (sama pola)
    // Department (sama pola)
    // Section (sama pola)

    // Tree
    GetOrganizationTree(ctx) ([]*Company, error)    // nested tree
}
```

---

### 3.5 Menu Domain (`domain/menu/`)

**Entity: `Menu`**

```
Fields:
  - id UUID
  - parentID *UUID (nullable, self-reference)
  - code string (^[A-Z][A-Z0-9_]*$)
  - title string
  - url string
  - iconName string
  - serviceName string
  - level int (1-3 CHECK)
  - sortOrder int
  - isVisible bool
  - isActive bool
  - permissions []*role.Permission (loaded separately)
  - children []*Menu (nested, loaded separately)
  - AuditInfo (embedded)

Domain Invariants:
  - level 1 → parentID MUST be nil
  - level 2,3 → parentID MUST be set
  - level MUST be 1-3
```

---

### 3.6 Audit Domain (`domain/audit/`)

**Entity: `Log`** — immutable audit record

```
Fields:
  - id UUID
  - eventType string (LOGIN/LOGOUT/LOGIN_FAILED/PASSWORD_RESET/
                      PASSWORD_CHANGE/2FA_ENABLED/2FA_DISABLED/
                      CREATE/UPDATE/DELETE/EXPORT/IMPORT)
  - tableName string
  - recordID *UUID
  - userID *UUID (nullable — pre-auth events)
  - username string
  - fullName string
  - ipAddress string
  - userAgent string
  - serviceName string
  - oldData, newData, changes interface{} (JSONB)
  - performedAt time.Time

Note: Tidak ada Update() atau Delete() method — audit logs bersifat immutable
```

---

### 3.7 Shared Domain (`domain/shared/`)

**Value Object: `AuditInfo`** — embedded di semua entitas

```go
type AuditInfo struct {
    CreatedAt time.Time
    CreatedBy UUID
    UpdatedAt time.Time
    UpdatedBy UUID
    DeletedAt *time.Time
    DeletedBy *UUID
}

Methods:
  - Update(updatedBy UUID)
  - SoftDelete(deletedBy UUID)
  - IsDeleted() bool
```

**Shared Domain Errors (22 error types):**

```
ErrNotFound, ErrAlreadyExists, ErrAlreadyDeleted, ErrNotActive,
ErrUnauthorized, ErrPermissionDenied, ErrInvalidCredentials,
ErrInvalidToken, ErrTokenRevoked, ErrTokenExpired,
ErrTwoFARequired, ErrTwoFAAlreadyEnabled, ErrTwoFANotEnabled,
ErrInvalid2FACode, ErrInvalidOTP, ErrAccountLocked,
ErrPasswordPolicy, ErrSystemRoleDelete, ErrMenuLevelInvalid,
ErrMenuParentRequired, ErrMenuParentInvalid, ErrInvalidInput
```

---

## 4. Application Layer — Detail Lengkap

### 4.1 Auth Service (`application/auth/service.go`)

Paling kompleks — single service struct dengan 11 operasi:

| Method | Deskripsi | Input | Output |
|--------|-----------|-------|--------|
| `Login` | Autentikasi user | username, password, totp_code?, device_info, ip, service | access_token, refresh_token, user_info |
| `Logout` | Invalidasi sesi | refresh_token | - |
| `RefreshToken` | Rotasi token pair | refresh_token | new access_token, new refresh_token |
| `ForgotPassword` | Kirim OTP reset | email | (side effect: email sent) |
| `VerifyResetOTP` | Validasi OTP | email, otp | reset_token (temp) |
| `ResetPassword` | Ganti password via token | reset_token, new_password | - |
| `UpdatePassword` | Ganti password (authenticated) | user_id, old_password, new_password | - |
| `Enable2FA` | Mulai setup TOTP | user_id | secret, qr_uri, recovery_codes |
| `Verify2FA` | Konfirmasi TOTP setup | user_id, totp_code | - |
| `Disable2FA` | Nonaktifkan TOTP | user_id, password, totp_code/recovery_code | - |
| `GetCurrentUser` | Info user dari token | user_id | user + roles + permissions |

**Login Flow (detailed):**

```
1. Rate limit check → Redis: iam:login_attempt:{username}
   → jika melewati threshold → return ErrAccountLocked

2. GetByUsername(db) → jika tidak ada → return ErrInvalidCredentials
   (pesan sama dengan wrong password — enumerate protection)

3. user.CanLogin() → cek: isActive, isLocked, lockedUntil <= now

4. password.Verify(input, hash)
   → primary: Argon2id verification
   → fallback: bcrypt legacy support (auto-migrate ke Argon2id setelah sukses)
   → gagal → user.RecordLoginFailure() → update DB → return ErrInvalidCredentials

5. Jika 2FA enabled → TOTP.Validate(secret, totpCode)
   → ±1 period clock-skew tolerance
   → gagal → return ErrInvalid2FACode / ErrTwoFARequired

6. jwt.GenerateTokenPair(userID, username, email, roles[], permissions[], services[])
   → access token: HS256, 15 min, {user_id, username, email, roles, permissions, service_access}
   → refresh token: HS256, 7 days, {user_id, jti (UUID)}

7. session.Create(userID, tokenHash, deviceInfo, ip, serviceName)
   → DB constraint: partial unique index user_id WHERE revoked_at IS NULL
   → existing active session → revoked (single-device policy)

8. cache.StoreSession(session, ttl=7days)

9. user.RecordLoginSuccess(ip) → update DB

10. audit.Create(LOGIN event, user info, ip, service)

11. Return: {access_token, refresh_token, expires_in, user:{id, username, email, roles[], permissions[]}}
```

---

### 4.2 Pattern Inconsistency: Handler vs Service

**Masalah kritis:** Ada dua pola berbeda di application layer:

**Pola A — Command/Query Handler (CQRS-lite):**  
Digunakan oleh: `user/`, `role/`, `permission/`, `session/`, `audit/`

```go
// Setiap use-case punya struct sendiri
type CreateHandler struct {
    userRepo    user.Repository
    roleRepo    role.Repository
    passSvc     password.Service
    auditRepo   audit.Repository
}

func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*CreateResult, error) { ... }
```

**Pola B — God Service:**  
Digunakan oleh: `auth/service.go`, `menu/service.go`, `organization/service.go`

```go
// Satu struct mengelola semua operasi domain
type Service struct {
    userRepo    user.Repository
    roleRepo    role.Repository
    sessionRepo session.Repository
    cacheRepo   session.CacheRepository
    jwtSvc      jwt.Service
    passSvc     password.Service
    totpSvc     totp.Service
    emailSvc    email.Service
    auditRepo   audit.Repository
    // ... 10+ dependencies
}

func (s *Service) Login(...)         { ... }
func (s *Service) Logout(...)        { ... }
func (s *Service) Enable2FA(...)     { ... }
// ... 11 methods
```

**Masalah:** Auth service memiliki 11 methods dengan 10+ dependencies — terlalu besar (God Service anti-pattern). Menu dan Organization service juga mengikuti pola ini, tidak konsisten dengan pola handler.

---

### 4.3 User Application Layer

**Handlers:** `create`, `get`, `list`, `update`, `delete`, `roles`, `permissions`

```
⚠ TEMUAN BUG: CreateCommand.PasswordHash menerima plain-text password
   dari request (req.GetPassword()), bukan hash.
   Field dinamai "PasswordHash" tapi isinya raw password.
   
   Handler perlu memanggil: hash, err := h.passwordSvc.Hash(cmd.Password)
   Jika tidak, password disimpan plaintext di database!
```

---

### 4.4 Session Application Layer

**Handlers:** `list`, `revoke`

```
⚠ GAP: GetCurrentSession di proto ada tapi tidak ada dedicated handler-nya.
   Handled inline di delivery layer.

⚠ GAP: session.CleanupExpired() ada di repository interface
   tapi tidak pernah dipanggil (tidak ada scheduler/cron job).
   Expired sessions menumpuk di database tanpa dibersihkan.
```

---

## 5. Infrastructure Layer — Detail Lengkap

### 5.1 JWT Service (`infrastructure/jwt/service.go`)

```
Algorithm: HS256 (HMAC-SHA256)
Issuer: "goapps-iam"

Access Token Claims:
  - sub: user_id (UUID)
  - username: string
  - email: string
  - roles: []string          → role codes
  - permissions: []string    → permission codes (SEMUA, bisa sangat besar)
  - service_access: []string → services yang boleh diakses
  - token_type: "access"
  - exp, iat, nbf, jti (UUID)

Refresh Token Claims:
  - sub: user_id
  - token_type: "refresh"
  - exp, iat, jti (UUID untuk revocation)

Secrets:
  - jwt.access_secret  (dari config)
  - jwt.refresh_secret (berbeda dari access secret ✅)

⚠ MASALAH KEAMANAN: HS256 tidak cocok untuk distributed multi-service.
  Setiap service yang perlu verify token HARUS memiliki access_secret.
  Untuk true SSO multi-service, RS256 atau ES256 (asymmetric) lebih aman:
  - IAM: private key (sign only)
  - Semua services lain: public key (verify only)
  - Saat ini: semua service share secret yang sama (symmetric key exposure risk)

⚠ MASALAH SKALABILITAS: Permissions di-embed ke dalam JWT.
  User dengan banyak permissions → JWT token sangat besar.
  Misal 50 permissions × 40 chars = 2KB per request header.
  Tidak cocok untuk production dengan ratusan permissions.
```

---

### 5.2 Password Service (`infrastructure/password/service.go`)

```
Primary: Argon2id
  - Memory: 64 MB (65536 KB)
  - Iterations: 3
  - Parallelism: 2
  - Salt: 16 bytes (random per hash)
  - Key Length: 32 bytes
  - Format: $argon2id$v=19$m=65536,t=3,p=2${base64salt}${base64hash}

Legacy Fallback: bcrypt ($2a$, $2b$, $2y$)
  - Auto-detect format dari prefix
  - Tidak ada auto-migration ke Argon2id setelah bcrypt login sukses ⚠

Password Policy (configurable):
  - Minimum length (default: 8)
  - Require uppercase ✅
  - Require lowercase ✅
  - Require number ✅
  - Require special character ❌ (tidak diimplementasi)

Constant-time comparison: ✅ (subtle.ConstantTimeCompare)
```

---

### 5.3 TOTP Service (`infrastructure/totp/service.go`)

```
Standard: RFC 6238 (TOTP)
Algorithm: HMAC-SHA1 (per RFC 6238)
Digits: 6
Period: 30 seconds
Clock skew tolerance: ±1 period (total 3 windows)
Secret encoding: Base32

Recovery Codes:
  - 8 kode per setup
  - 10 karakter random hex
  - Disimpan sebagai SHA256 hash di database (user_recovery_codes)
  - Single-use (ditandai used_at setelah dipakai)

⚠ IMPLEMENTASI CUSTOM: TOTP diimplementasi dari scratch tanpa library eksternal.
  Ini meningkatkan risiko bug kriptografi. Sebaiknya gunakan library teruji
  seperti github.com/pquerna/otp atau github.com/xlzd/gohotp.

⚠ QR Code URI: Service membuat otpauth:// URI tapi tidak generate actual QR image.
  Frontend harus render QR dari URI tersebut (menggunakan library JS seperti qrcode.js).
```

---

### 5.4 Redis Cache (`infrastructure/redis/cache.go`)

```
IAM menggunakan Redis DB 1

Key Namespaces:
  iam:session:{sessionID}       TTL: refresh_token_ttl (7 days)
  iam:blacklist:{jti}           TTL: sisa lifetime token
  iam:otp:{userID}              TTL: 5 menit
  iam:reset:{token}             TTL: 10 menit
  iam:2fa_setup:{userID}        TTL: 10 menit
  iam:2fa_codes:{userID}        TTL: 10 menit
  iam:login_attempt:{username}  TTL: lock duration

⚠ MASALAH: 2FA recovery codes disimpan di Redis sementara menunggu verifikasi.
  Disimpan sebagai comma-separated string: "code1,code2,code3,...".
  Jika Redis restart, setup 2FA yang pending hilang tanpa notifikasi ke user.
  Sebaiknya disimpan di DB sementara (dengan status pending), bukan Redis.

⚠ Finance service (Redis DB 0) membaca iam:blacklist:{jti} di DB 1 untuk
  validasi token. Ini coupling langsung antar service via shared Redis namespace.
  Lebih aman menggunakan token introspection endpoint di IAM.
```

---

### 5.5 Email Service (`infrastructure/email/service.go`)

```
Protocol: SMTP dengan optional TLS
Dev: Mailpit (port 1025, web UI port 8025)

Methods:
  - SendOTP(email, otp, expiryMinutes)      → password reset email
  - Send2FANotification(email, action)       → 2FA change notification

⚠ TIDAK ADA: Welcome email saat user baru dibuat
⚠ TIDAK ADA: Account locked notification
⚠ TIDAK ADA: Email template engine (HTML email)
⚠ TIDAK ADA: Email queue/retry mechanism (sync SMTP, blocking)
```

---

### 5.6 Database Connection (`infrastructure/postgres/connection.go`)

```
Driver: github.com/jackc/pgx/v5/stdlib (SCRAM-SHA-256)
Pool config:
  - MaxOpenConns: 25
  - MaxIdleConns: 5
  - ConnMaxLifetime: 5 menit

Transaction helper:
  DB.Transaction(ctx, fn) → auto rollback on error

Migrations: golang-migrate (dijalankan via Makefile atau manual)
```

---

## 6. Delivery Layer — Detail Lengkap

### 6.1 gRPC Server (`delivery/grpc/server.go`)

```
Port: 50052 (gRPC)
Port: 8081 (HTTP gateway)

gRPC Server Options:
  - MaxRecvMsgSize: 10 MB
  - MaxSendMsgSize: 10 MB
  - KeepAlive: enabled
  - TLS: tidak diimplementasi ⚠ (plain gRPC, cocok untuk internal only)

Services Registered (11 total):
  1. AuthService
  2. UserService
  3. RoleService
  4. PermissionService
  5. SessionService
  6. AuditService
  7. MenuService
  8. CompanyService
  9. DivisionService
  10. DepartmentService
  11. SectionService
```

### 6.2 Interceptor Chain (diaplikasikan ke SEMUA unary calls)

```
Order (dari luar ke dalam):
1. StructuredErrorInterceptor  → wrap gRPC errors ke typed BaseResponse
2. RecoveryInterceptor          → panic → codes.Internal
3. RequestIDInterceptor         → UUID request ID dari/ke x-request-id header
4. TracingInterceptor            → OpenTelemetry span per method
5. MetricsInterceptor            → Prometheus counter + histogram per method
6. RateLimitInterceptor          → token-bucket (global only, per-method TIDAK dipakai ⚠)
7. AuthInterceptor               → JWT validation + blacklist (skip: public methods)
8. PermissionInterceptor         → RBAC check (skip: public methods + non-mapped → DENY)
9. LoggingInterceptor            → structured request/response logging
10. TimeoutInterceptor           → 30s default timeout
```

**Public Methods (skip AuthInterceptor):**

```go
var publicMethods = map[string]bool{
    "/iam.v1.AuthService/Login":           true,
    "/iam.v1.AuthService/ForgotPassword":  true,
    "/iam.v1.AuthService/VerifyResetOTP":  true,
    "/iam.v1.AuthService/ResetPassword":   true,
    "/iam.v1.AuthService/RefreshToken":    true,
    "/iam.v1.AuthService/Logout":          true,
}
```

**Permission Map (sample):**

```go
var methodPermissions = map[string]string{
    "/iam.v1.UserService/CreateUser":        "iam.user.account.create",
    "/iam.v1.UserService/GetUser":           "iam.user.account.view",
    "/iam.v1.UserService/ListUsers":         "iam.user.account.view",
    "/iam.v1.UserService/UpdateUser":        "iam.user.account.update",
    "/iam.v1.UserService/DeleteUser":        "iam.user.account.delete",
    "/iam.v1.RoleService/CreateRole":        "iam.user.role.create",
    "/iam.v1.RoleService/ListRoles":         "iam.user.role.view",
    // ... (method yang tidak ada di map → DEFAULT DENY)
}
```

**⚠ MASALAH:** Beberapa method yang baru ditambahkan (Export/Import stubs) tidak ada di `methodPermissions` map → auto-deny bahkan untuk SUPER_ADMIN bukan (karena SUPER_ADMIN bypass diprioritaskan).

---

### 6.3 HTTP Gateway (`delivery/httpdelivery/gateway.go`)

```
Port: 8081
Prefix: /api/

Endpoints tambahan:
  GET /healthz    → health check
  GET /readyz     → readiness check
  GET /livez      → liveness check
  GET /metrics    → Prometheus metrics
  GET /swagger/   → Swagger UI
  GET /swagger.json

CORS: configured via github.com/rs/cors
  ⚠ AllowedOrigins tidak dikonfigurasi dengan benar di config.yaml
    Default mungkin terlalu permissive

Proto Validation: buf.build/go/protovalidate
  ✅ Validasi proto message sebelum handler dipanggil
```

---

### 6.4 Rate Limiter (`delivery/grpc/rate_limiter.go`)

```
Implementation: Token bucket (per-service global)
Library: golang.org/x/time/rate

Config:
  - Global rate: dari config.yaml
  - Per-method override map (e.g., auth endpoints: 5 rps)

⚠ BUG KRITIS: methodLimits map DIDEFINISIKAN tapi TIDAK DIGUNAKAN di interceptor.
  RateLimitInterceptor hanya panggil r.globalLimiter.Allow() untuk semua methods.
  Auth endpoints yang seharusnya dibatasi 5 rps tidak berbeda dari endpoint lain.
  
  Seharusnya:
    if limiter, ok := r.methodLimits[info.FullMethod]; ok {
        if !limiter.Allow() { return ErrRateLimit }
    }
    if !r.globalLimiter.Allow() { return ErrRateLimit }

⚠ Rate limiter bersifat in-memory per-instance.
  Dalam deployment multi-instance, rate limit tidak di-share antar pod.
  Sebaiknya gunakan Redis-based distributed rate limiter.

⚠ Rate limiting berdasarkan global, bukan per-IP atau per-user.
  DDoS dari satu IP bisa block semua user lain.
```

---

## 7. Database Schema — Detail Lengkap

### 7.1 Migration 000001 — Organization Tables

```sql
-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- mst_company
CREATE TABLE mst_company (
    company_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    company_code    VARCHAR(20) UNIQUE NOT NULL
                    CHECK (company_code ~ '^[A-Z][A-Z0-9_]*$'),
    company_name    VARCHAR(200) NOT NULL,
    description     TEXT,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by      UUID NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by      UUID NOT NULL,
    deleted_at      TIMESTAMPTZ,
    deleted_by      UUID
);

-- Partial indexes (WHERE deleted_at IS NULL) untuk soft-delete correctness
CREATE UNIQUE INDEX ... ON mst_company(company_code) WHERE deleted_at IS NULL;
CREATE INDEX ... USING GIN (company_name gin_trgm_ops) WHERE deleted_at IS NULL;

-- Division, Department, Section (pola serupa dengan FK ke parent)
```

### 7.2 Migration 000002 — User Tables

```sql
CREATE TABLE mst_user (
    user_id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username                VARCHAR(50) UNIQUE NOT NULL,
    email                   VARCHAR(255) UNIQUE NOT NULL,
    password_hash           VARCHAR(255) NOT NULL,
    is_active               BOOLEAN NOT NULL DEFAULT TRUE,
    is_locked               BOOLEAN NOT NULL DEFAULT FALSE,
    failed_login_attempts   INT NOT NULL DEFAULT 0,
    locked_until            TIMESTAMPTZ,
    two_factor_enabled      BOOLEAN NOT NULL DEFAULT FALSE,
    two_factor_secret       VARCHAR(64),    -- ⚠ Plaintext! Should be encrypted
    last_login_at           TIMESTAMPTZ,
    last_login_ip           VARCHAR(45),
    password_changed_at     TIMESTAMPTZ,
    ... (audit fields)
);

CREATE TABLE mst_user_detail (
    detail_id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id                 UUID NOT NULL REFERENCES mst_user(user_id) ON DELETE CASCADE,
    section_id              UUID REFERENCES mst_section(section_id) ON DELETE SET NULL,
    employee_code           VARCHAR(50) UNIQUE NOT NULL,
    full_name               VARCHAR(200) NOT NULL,
    first_name              VARCHAR(100),
    last_name               VARCHAR(100),
    phone                   VARCHAR(20),
    profile_picture_url     VARCHAR(500),
    position                VARCHAR(100),
    date_of_birth           DATE,
    address                 TEXT,
    extra_data              JSONB DEFAULT '{}'
);
```

### 7.3 Migration 000003 — Auth Tables

```sql
CREATE TABLE user_sessions (
    session_id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES mst_user(user_id) ON DELETE CASCADE,
    refresh_token_hash  VARCHAR(64) NOT NULL,  -- SHA256 hex
    device_info         VARCHAR(500),
    ip_address          VARCHAR(45),
    service_name        VARCHAR(100),
    expires_at          TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Single-device policy enforced at DB level!
CREATE UNIQUE INDEX idx_user_active_session
    ON user_sessions(user_id)
    WHERE revoked_at IS NULL;

-- ⚠ password_reset_tokens table ada di migration tapi TIDAK DIPAKAI oleh kode aplikasi
CREATE TABLE password_reset_tokens ( ... );

-- ⚠ api_keys table ada di migration tapi TIDAK DIPAKAI (no handler, no repo)
CREATE TABLE api_keys (
    key_id                  UUID,
    user_id                 UUID,
    key_name                VARCHAR(100),
    key_hash                VARCHAR(64),
    key_prefix              VARCHAR(16),    -- first 16 chars untuk identification
    allowed_ips             INET[],
    allowed_scopes          VARCHAR(100)[],
    service_name            VARCHAR(100),
    rate_limit_per_minute   INT,
    expires_at              TIMESTAMPTZ,
    last_used_at            TIMESTAMPTZ,
    is_active               BOOLEAN,
    revoked_at              TIMESTAMPTZ
);
```

### 7.4 Migration 000004 — RBAC Tables

```sql
CREATE TABLE mst_role (
    role_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    role_code       VARCHAR(50) UNIQUE NOT NULL,
    role_name       VARCHAR(100) NOT NULL,
    description     TEXT,
    is_system       BOOLEAN NOT NULL DEFAULT FALSE,  -- protected from deletion
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    ... (audit fields + soft delete)
);

CREATE TABLE mst_permission (
    permission_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    permission_code VARCHAR(100) UNIQUE NOT NULL,
    -- Format: service.module.entity.action (4 parts, regex-validated)
    permission_name VARCHAR(200) NOT NULL,
    service_name    VARCHAR(50) NOT NULL,   -- iam, finance, hr, etc.
    module_name     VARCHAR(50) NOT NULL,
    action_type     VARCHAR(20) NOT NULL
                    CHECK (action_type IN ('view','create','update','delete','export','import')),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE
);

-- Junction tables dengan composite unique constraints
CREATE TABLE user_roles (
    user_id UUID REFERENCES mst_user, role_id UUID REFERENCES mst_role,
    UNIQUE(user_id, role_id)
);
CREATE TABLE role_permissions (
    role_id UUID REFERENCES mst_role, permission_id UUID REFERENCES mst_permission,
    UNIQUE(role_id, permission_id)
);
CREATE TABLE user_permissions (
    user_id UUID REFERENCES mst_user, permission_id UUID REFERENCES mst_permission,
    UNIQUE(user_id, permission_id)
);
```

### 7.5 Migration 000005 — Menu Tables

```sql
CREATE TABLE mst_menu (
    menu_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parent_id       UUID REFERENCES mst_menu(menu_id) ON DELETE SET NULL,
    menu_code       VARCHAR(50) UNIQUE NOT NULL,
    menu_title      VARCHAR(200) NOT NULL,
    menu_url        VARCHAR(500),
    icon_name       VARCHAR(100),
    service_name    VARCHAR(50),
    menu_level      INT NOT NULL CHECK (menu_level BETWEEN 1 AND 3),
    sort_order      INT NOT NULL DEFAULT 0,
    is_visible      BOOLEAN NOT NULL DEFAULT TRUE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    ... (audit fields + soft delete)
);

-- Level constraint via CHECK:
ALTER TABLE mst_menu ADD CONSTRAINT check_level_parent
    CHECK (
        (menu_level = 1 AND parent_id IS NULL) OR
        (menu_level > 1 AND parent_id IS NOT NULL)
    );

CREATE TABLE menu_permissions (
    menu_id         UUID REFERENCES mst_menu,
    permission_id   UUID REFERENCES mst_permission,
    PRIMARY KEY (menu_id, permission_id)
);
```

---

## 8. Alur Autentikasi & Otorisasi

### 8.1 Login Flow

```
Client → POST /api/v1/iam/auth/login
  Body: { username, password, totp_code?, device_info, service }

┌─────────────────────────────────────────────────────────────────┐
│ 1. Rate limit check (Redis)                                      │
│    Key: iam:login_attempt:{username}                             │
│    Threshold: configurable (default: 5 attempts/15 min)         │
│    → exceeded: return 429 + ErrAccountLocked                    │
├─────────────────────────────────────────────────────────────────┤
│ 2. User lookup (PostgreSQL)                                      │
│    GetByUsername() → null → return 401 ErrInvalidCredentials    │
│    (sama message seperti wrong password — timing attack defense) │
├─────────────────────────────────────────────────────────────────┤
│ 3. Account state check                                           │
│    user.CanLogin() → isActive=false → 403 ErrNotActive          │
│                   → isLocked=true, lockedUntil>now → 423 Locked │
├─────────────────────────────────────────────────────────────────┤
│ 4. Password verification                                         │
│    Argon2id verify (atau bcrypt fallback)                        │
│    → fail → RecordLoginFailure() → update DB → return 401       │
├─────────────────────────────────────────────────────────────────┤
│ 5. 2FA check (jika enabled)                                      │
│    → totp_code kosong → return 200 + ErrTwoFARequired           │
│    → totp_code salah → return 401 ErrInvalid2FACode              │
├─────────────────────────────────────────────────────────────────┤
│ 6. Token generation                                              │
│    GenerateTokenPair(userID, username, email, roles, perms)      │
│    access: HS256, 15 min                                         │
│    refresh: HS256, 7 days, JTI=UUID                              │
├─────────────────────────────────────────────────────────────────┤
│ 7. Session creation                                              │
│    DB: INSERT INTO user_sessions (dengan partial unique index)   │
│    → revoke existing active session first                        │
│    Redis: StoreSession (TTL=7days)                               │
├─────────────────────────────────────────────────────────────────┤
│ 8. RecordLoginSuccess(ip) → DB update                            │
│ 9. AuditLog: LOGIN event                                         │
│ 10. Response: {access_token, refresh_token, expires_in, user}   │
└─────────────────────────────────────────────────────────────────┘
```

### 8.2 Token Refresh Flow

```
Client → POST /api/v1/iam/auth/refresh
  Body: { refresh_token }

1. Parse & validate refresh JWT
2. Check token_type == "refresh"
3. Check blacklist (Redis: iam:blacklist:{jti})
4. GetByTokenID(jti) → DB lookup → session must be active + not expired
5. Generate new TokenPair
6. BlacklistToken(old_jti, remaining_ttl)
7. Update session.refresh_token_hash in DB
8. Update Redis session cache
9. Return: {access_token, refresh_token, expires_in}
```

### 8.3 Per-Request Authorization Flow

```
Client Request → gRPC → [Interceptor Chain]

1. AuthInterceptor:
   - Extract "authorization: Bearer {token}" dari gRPC metadata
   - ParseAndValidate(token) → check exp, iss, typ
   - IsTokenBlacklisted(jti) → Redis check (fail-open jika Redis down)
   - Inject claims ke context: userID, username, email, roles, permissions

2. PermissionInterceptor:
   - Get required permission dari methodPermissions[fullMethod]
   - Method tidak ada di map → DENY (default-deny ✅)
   - roles contains "SUPER_ADMIN" → ALLOW (bypass semua permission check)
   - permissions contains required_permission → ALLOW
   - Else → DENY dengan codes.PermissionDenied
```

### 8.4 Cross-Service Token Validation (Finance Service)

```
Finance Service AuthInterceptor:
1. Extract token dari metadata
2. ParseAndValidate menggunakan jwt.access_secret (SAMA dengan IAM)
3. Check blacklist di IAM Redis DB 1: iam:blacklist:{jti}
   → Redis key: iam:blacklist:{jti}
   
⚠ Problem: Finance service memiliki copy dari access_secret.
  Jika secret perlu di-rotate, SEMUA services harus restart.
  Asymmetric key (RS256/ES256) akan menghilangkan masalah ini.

⚠ Problem: Direct Redis coupling antar services.
  Solusi yang lebih bersih: IAM expose endpoint ValidateToken gRPC
  yang dipanggil oleh service lain (token introspection).
```

---

## 9. gRPC Services & REST Gateway

### 9.1 IAM gRPC Services — Semua Endpoints

| Service | Method | Auth | Permission | Status |
|---------|--------|------|-----------|--------|
| **AuthService** | Login | ❌ Public | - | ✅ |
| | Logout | ❌ Public | - | ✅ |
| | RefreshToken | ❌ Public | - | ✅ |
| | ForgotPassword | ❌ Public | - | ✅ |
| | VerifyResetOTP | ❌ Public | - | ✅ |
| | ResetPassword | ❌ Public | - | ✅ |
| | GetCurrentUser | ✅ JWT | - | ✅ |
| | UpdatePassword | ✅ JWT | - | ✅ |
| | Enable2FA | ✅ JWT | - | ✅ |
| | Verify2FA | ✅ JWT | - | ✅ |
| | Disable2FA | ✅ JWT | - | ✅ |
| **UserService** | CreateUser | ✅ JWT | iam.user.account.create | ✅ |
| | GetUser | ✅ JWT | iam.user.account.view | ✅ |
| | GetUserDetail | ✅ JWT | iam.user.account.view | ✅ |
| | ListUsers | ✅ JWT | iam.user.account.view | ✅ |
| | UpdateUser | ✅ JWT | iam.user.account.update | ✅ |
| | UpdateUserDetail | ✅ JWT | iam.user.account.update | ✅ |
| | DeleteUser | ✅ JWT | iam.user.account.delete | ✅ |
| | AssignUserRoles | ✅ JWT | iam.user.role.assign | ✅ |
| | RemoveUserRoles | ✅ JWT | iam.user.role.assign | ✅ |
| | AssignUserPermissions | ✅ JWT | iam.user.permission.assign | ✅ |
| | RemoveUserPermissions | ✅ JWT | iam.user.permission.assign | ✅ |
| | GetUserRolesAndPermissions | ✅ JWT | iam.user.account.view | ✅ |
| | ExportUsers | ✅ JWT | iam.user.account.export | ⚠ 501 Stub |
| | ImportUsers | ✅ JWT | iam.user.account.import | ⚠ 501 Stub |
| | DownloadTemplate | ✅ JWT | - | ⚠ 501 Stub |
| **RoleService** | CreateRole | ✅ JWT | iam.user.role.create | ✅ |
| | GetRole | ✅ JWT | iam.user.role.view | ✅ |
| | ListRoles | ✅ JWT | iam.user.role.view | ✅ |
| | UpdateRole | ✅ JWT | iam.user.role.update | ✅ |
| | DeleteRole | ✅ JWT | iam.user.role.delete | ✅ |
| | GetRolePermissions | ✅ JWT | iam.user.role.view | ✅ |
| | AssignRolePermissions | ✅ JWT | iam.user.role.assign | ✅ |
| | RemoveRolePermissions | ✅ JWT | iam.user.role.assign | ✅ |
| | ExportRoles | ✅ JWT | iam.user.role.export | ⚠ 501 Stub |
| | ImportRoles | ✅ JWT | iam.user.role.import | ⚠ 501 Stub |
| **PermissionService** | CreatePermission | ✅ JWT | iam.user.permission.create | ✅ |
| | ListPermissions | ✅ JWT | iam.user.permission.view | ✅ |
| | UpdatePermission | ✅ JWT | iam.user.permission.update | ✅ |
| | DeletePermission | ✅ JWT | iam.user.permission.delete | ✅ |
| | GetPermissionsByService | ✅ JWT | iam.user.permission.view | ✅ |
| **SessionService** | GetCurrentSession | ✅ JWT | - | ✅ |
| | ListActiveSessions | ✅ JWT | - | ✅ |
| | RevokeSession | ✅ JWT | - | ✅ |
| **AuditService** | GetAuditLog | ✅ JWT | iam.audit.log.view | ✅ |
| | ListAuditLogs | ✅ JWT | iam.audit.log.view | ✅ |
| | GetAuditSummary | ✅ JWT | iam.audit.log.view | ✅ |
| | ExportAuditLogs | ✅ JWT | iam.audit.log.export | ⚠ 501 Stub |
| **MenuService** | CreateMenu | ✅ JWT | iam.system.menu.create | ✅ |
| | GetMenu | ✅ JWT | iam.system.menu.view | ✅ |
| | ListMenus | ✅ JWT | iam.system.menu.view | ✅ |
| | UpdateMenu | ✅ JWT | iam.system.menu.update | ✅ |
| | DeleteMenu | ✅ JWT | iam.system.menu.delete | ✅ |
| | GetMenuTree | ✅ JWT | - | ✅ |
| | GetFullMenuTree | ✅ JWT | - | ✅ |
| | AssignMenuPermissions | ✅ JWT | iam.system.menu.update | ✅ |
| | ReorderMenus | ✅ JWT | iam.system.menu.update | ✅ |
| **Company/Division/Department/Section** | All CRUD | ✅ JWT | iam.organization.*.* | ✅ |
| | All Export/Import | ✅ JWT | iam.organization.*.export/import | ⚠ 501 Stub |
| **OrganizationService** | GetOrganizationTree | ✅ JWT | - | ✅ |

---

## 10. Keamanan & Kriptografi

### Yang Sudah Baik ✅

| Aspek | Implementasi |
|-------|-------------|
| Password hashing | Argon2id (state-of-the-art) |
| Constant-time comparison | `subtle.ConstantTimeCompare` |
| Separate JWT secrets | Access ≠ Refresh secret |
| JWT blacklist | Redis-based dengan TTL |
| Brute force protection | Rate limit + account lockout |
| Single-device session | Partial unique index di DB |
| OTP expiry | Redis TTL 5 menit |
| 2FA recovery codes | SHA256-hashed, single-use |
| SQL injection | Parameterized queries (`$1, $2`) |
| Soft delete | Data tidak benar-benar dihapus |
| Audit logging | Semua operasi tercatat |
| Account enumeration defense | Pesan error sama untuk invalid user/password |

### Yang Bermasalah / Perlu Perhatian ⚠

| Masalah | Tingkat Keparahan | Detail |
|---------|-------------------|--------|
| HS256 symmetric key | 🔴 High | Semua service harus punya secret yang sama |
| TOTP secret plaintext | 🔴 High | `two_factor_secret` tidak dienkripsi di DB |
| Permissions di JWT | 🟡 Medium | Token besar, tidak bisa revoke per-permission |
| Custom TOTP impl | 🟡 Medium | Risiko bug kriptografi (gunakan library teruji) |
| No TLS on gRPC | 🟡 Medium | Plain gRPC (oke untuk internal, not for internet) |
| Rate limiter per-method bug | 🟡 Medium | methodLimits map tidak dipakai |
| Redis coupling cross-service | 🟡 Medium | Finance langsung baca IAM Redis namespace |
| No special char password | 🟢 Low | Policy tidak require special character |
| No CSRF protection | 🟢 Low | Tidak ada CSRF token untuk HTTP gateway |
| Fail-open blacklist | 🟢 Low | Intentional, mitigated by short 15min access TTL |

---

## 11. Observability (Logging, Metrics, Tracing)

### Logging

```
Library: github.com/rs/zerolog
Format: JSON structured
Fields: time, level, service, request_id, method, duration, error, user_id

Output: stdout/stderr (untuk container log aggregation)
Levels: trace, debug, info, warn, error, fatal

⚠ Tidak ada log sampling untuk high-traffic (misal ListUsers bisa flood logs)
⚠ Response body tidak di-log (untuk keamanan) ✅ tapi request body mungkin berisi PII
```

### Metrics (Prometheus)

```
Endpoint: GET /metrics
Library: github.com/prometheus/client_golang

Counters/Histograms per method:
  - iam_grpc_requests_total{method, status}
  - iam_grpc_request_duration_seconds{method}
  
⚠ Tidak ada business metrics:
  - active_sessions_count
  - login_success_rate
  - 2fa_adoption_rate
  - locked_accounts_count
```

### Tracing

```
Library: go.opentelemetry.io/otel
Exporter: Jaeger (OTLP/gRPC)
Jaeger UI: http://localhost:16686

Per-request span dengan:
  - method name
  - request_id
  - status code
  
⚠ Tidak ada trace propagation ke downstream services
  (Finance tidak propagate trace ID ke IAM for token introspection)
```

---

## 12. Testing Coverage

### Unit Tests

| File | Coverage | Quality |
|------|----------|---------|
| `domain/user/entity_test.go` | ✅ Good | Konstruktor + domain methods |
| `domain/role/entity_test.go` | ✅ Good | Role + Permission entity |
| `domain/menu/entity_test.go` | ✅ Good | Level constraint |
| `domain/organization/entity_test.go` | ✅ Good | 4 entitas org |
| `infrastructure/password/service_test.go` | ✅ Good | Hash + verify + policy |
| `infrastructure/postgres/user_repository_test.go` | ✅ Good | Integration test |
| `infrastructure/postgres/role_repository_test.go` | ✅ Good | Integration test |
| `delivery/grpc/auth_interceptor_test.go` | ✅ Good | JWT validation |
| `delivery/grpc/permission_interceptor_test.go` | ✅ Good | RBAC logic |
| `delivery/grpc/error_response_test.go` | ✅ Good | Error mapping |

### Integration Tests

```
tests/e2e/auth_test.go   → Login, Logout, Refresh, 2FA flow
tests/e2e/role_test.go   → CRUD roles
tests/e2e/user_test.go   → CRUD users
```

### Coverage Gaps

```
❌ application/auth/service.go — tidak ada unit test
❌ application/user/ handlers — tidak ada unit test
❌ application/organization/service.go — tidak ada test
❌ application/menu/service.go — tidak ada test
❌ infrastructure/jwt/service.go — tidak ada unit test
❌ infrastructure/totp/service.go — tidak ada unit test
❌ infrastructure/email/service.go — tidak ada test
❌ infrastructure/redis/cache.go — tidak ada unit test
❌ delivery/grpc/*_handler.go — tidak ada unit test per handler
```

---

## 13. Temuan: Bug & Masalah Kritis

### BUG-001: Rate Limiter Per-Method Tidak Berfungsi

**File:** `services/iam/internal/delivery/grpc/rate_limiter.go`  
**Severity:** 🔴 High  
**Deskripsi:** Map `methodLimits` didefinisikan (auth endpoints: 5 rps) namun interceptor hanya memanggil `r.globalLimiter.Allow()`. Auth endpoints seharusnya lebih ketat dilindungi dari brute force.

---

### BUG-002: Session Token Hash — Inconsistency

**File:** `services/iam/internal/application/auth/service.go` + `infrastructure/postgres/session_repository.go`  
**Severity:** 🔴 High  
**Deskripsi:** Session dibuat dengan `hashToken(tokenPair.TokenID)` — jadi yang di-hash adalah **JTI UUID** bukan full refresh JWT. Namun `session.ValidateToken(tokenString)` di domain meng-hash full token string. `GetByRefreshToken(fullJWT)` tidak akan pernah match — hanya `GetByTokenID(jti)` yang bekerja.

---

### BUG-003: CreateUser — Possible Plaintext Password Storage

**File:** `services/iam/internal/application/user/create_handler.go`  
**Severity:** 🔴 Critical  
**Deskripsi:** `CreateCommand.PasswordHash` menerima `req.GetPassword()` langsung. Perlu diverifikasi apakah `Handle()` memanggil `h.passwordSvc.Hash()` sebelum meneruskan ke domain. Jika tidak, password disimpan plaintext.

---

### BUG-004: session.CleanupExpired() Tidak Pernah Dipanggil

**File:** `services/iam/internal/domain/session/repository.go`  
**Severity:** 🟡 Medium  
**Deskripsi:** Interface mendefinisikan `CleanupExpired(ctx)` tapi tidak ada scheduler, cron job, atau kode yang memanggilnya. DB akan terus terisi expired sessions.

---

### BUG-005: password_reset_tokens Table Tidak Digunakan

**File:** `services/iam/migrations/postgres/000003_create_auth_tables.up.sql`  
**Severity:** 🟡 Medium  
**Deskripsi:** Tabel `password_reset_tokens` ada di migration tapi tidak ada repository atau kode yang menulis ke sana. OTP flow hanya menggunakan Redis. Tabel ini adalah dead schema.

---

### BUG-006: bcrypt Legacy — Tidak Ada Auto-Migration

**File:** `services/iam/internal/infrastructure/password/service.go`  
**Severity:** 🟢 Low  
**Deskripsi:** Service mendukung bcrypt untuk legacy tapi tidak ada mekanisme otomatis untuk meng-upgrade hash ke Argon2id setelah login sukses. Semua user legacy tetap bcrypt selamanya.

---

## 14. Temuan: Gap Fungsionalitas

### GAP-001: API Key Authentication — Schema Ada, Implementasi Tidak Ada

**Severity:** 🟡 Medium  
**Deskripsi:** Tabel `api_keys` dirancang dengan sempurna (scoped permissions, IP whitelist, rate limiting) namun tidak ada:
- Repository interface
- Application handler
- gRPC endpoint
- Interceptor untuk API key auth

**Impact:** Service-to-service authentication (misalnya HR service → IAM) hanya bisa menggunakan JWT user token, bukan dedicated API key.

---

### GAP-002: Token Introspection Endpoint — Tidak Ada

**Severity:** 🔴 High (untuk SSO multi-service)  
**Deskripsi:** Tidak ada endpoint `/introspect` atau gRPC method `ValidateToken` di IAM. Service lain (Finance, HR) tidak bisa memvalidasi token melalui IAM — mereka harus melakukan validasi sendiri dengan shared secret, yang berarti:
- Secret harus didistribusikan ke semua services
- Tidak ada centralized revocation notification
- Finance hardcode ke Redis IAM untuk blacklist check

---

### GAP-003: Export/Import Bulk — 501 di Semua Entitas

**Severity:** 🟡 Medium  
**Detail:** 29 endpoint mengembalikan 501 Not Implemented:
- Users: Export, Import, Template
- Roles: Export, Import, Template
- Permissions: Export, Import, Template
- Menu: Export, Import, Template
- Company/Division/Department/Section: masing-masing Export, Import, Template
- Audit: ExportLogs

---

### GAP-004: Email System — Terbatas

**Severity:** 🟡 Medium  
**Yang Tidak Ada:**
- Welcome email saat user baru dibuat
- Account locked notification email
- Suspicious login (new IP/device) notification
- HTML email templates (hanya plain text)
- Email queue dengan retry (blocking SMTP saat ini)

---

### GAP-005: Profile Picture Upload — URL Only, No Storage

**Severity:** 🟢 Low  
**Deskripsi:** `profile_picture_url` menyimpan URL tapi tidak ada endpoint untuk upload gambar. Frontend harus menggunakan S3/GCS terpisah dan paste URL. Belum ada validasi format URL atau ukuran.

---

### GAP-006: Password Expiry Policy — Tidak Ada

**Severity:** 🟡 Medium  
**Deskripsi:** Field `password_changed_at` ada di DB tapi tidak ada:
- Config untuk password expiry duration
- Check saat login (password expired → force change)
- Notification email sebelum expiry

---

### GAP-007: Refresh Token Rotation Invalidation Chain

**Severity:** 🟡 Medium  
**Deskripsi:** Saat refresh dilakukan, token lama di-blacklist di Redis. Namun jika attacker mencuri refresh token dan melakukan refresh sebelum user, user akan mendapat error karena token sudah di-blacklist. Tidak ada mechanism untuk mendeteksi bahwa refresh token telah digunakan (token reuse detection).

---

### GAP-008: Multi-Tenant / Organizational Isolation

**Severity:** 🟡 Medium  
**Deskripsi:** Meskipun ada struktur Company → Division → Department → Section, tidak ada:
- Row-level security per company
- Multi-tenant permission scoping
- User dibatasi hanya bisa lihat data company mereka sendiri

---

### GAP-009: CI/CD Pipeline — IAM Belum Ada

**Severity:** 🟡 Medium  
**Deskripsi:** Finance service punya `.github/workflows/finance-service.yml` tapi IAM tidak memiliki CI/CD pipeline sama sekali.

---

### GAP-010: Kubernetes Manifests — IAM Tidak Lengkap

**Severity:** 🟢 Low  
**Deskripsi:** Finance memiliki lengkap: deployment, service, configmap, secret template, HPA, PDB, NetworkPolicy, RBAC. IAM hanya memiliki `deployments/docker-compose.yaml`.

---

## 15. Temuan: Inkonsistensi Pola

### INCON-001: Application Layer Pattern Tidak Konsisten

**Deskripsi:** Dua pola berbeda dalam satu service:
- Pola A (CQRS Handler): `user/`, `role/`, `permission/`, `session/`, `audit/`
- Pola B (God Service): `auth/service.go`, `menu/service.go`, `organization/service.go`

**Seharusnya:** Semua domain menggunakan pola yang sama (A atau B, konsisten).

---

### INCON-002: Auth Service Terlalu Besar (God Object)

**Deskripsi:** `application/auth/service.go` mengelola 11 operasi berbeda dengan 10+ dependencies.  
**Masalah:** Sulit di-test, sulit di-maintain.  
**Solusi:** Pisahkan menjadi: `AuthHandler` (login/logout/refresh), `PasswordHandler` (forgot/reset/update), `TwoFAHandler` (enable/verify/disable).

---

### INCON-003: Finance vs IAM Audit Log Format Berbeda

**Deskripsi:** IAM audit log (`audit_logs`) sangat lengkap dengan eventType, username, fullName, userAgent, dll. Finance audit log hanya mencatat `table_name, action, old_data, new_data`. Tidak ada standar audit log antar services.

---

### INCON-004: Menu dan Organization Belum Ikuti Handler Pattern

**Deskripsi:** `menu/service.go` dan `organization/service.go` masih menggunakan service struct besar, berbeda dari domain lain yang sudah memakai handler per use-case.

---

## 16. Temuan: Technical Debt

### DEBT-001: TOTP Custom Implementation

Kode TOTP ditulis dari scratch. Library `github.com/pquerna/otp` lebih teruji, mendukung lebih banyak TOTP variants, dan digunakan oleh ribuan production systems.

### DEBT-002: lib/pq Dependency di Finance Service

Finance `go.mod` menginclude `github.com/lib/pq` (legacy postgres driver) padahal sudah menggunakan `pgx/v5`. Dependency tidak terpakai.

### DEBT-003: Root README Tidak Mention IAM Service

Root `README.md` hanya mendokumentasikan Finance service. Tidak ada dokumentasi untuk IAM service di root README.

### DEBT-004: Deploy Directory Kosong

`/deploy/` directory di root ada tapi kosong. IAM Kubernetes manifests ada di `services/iam/deployments/` tapi tidak selengkap Finance.

### DEBT-005: No `.gitignore` untuk IAM Service

Finance service punya `.gitignore`, IAM service tidak.

### DEBT-006: Seeds — Tidak Ada Dokumentasi

`seeds/main.go` ada tapi tidak terdokumentasi: apa yang di-seed, bagaimana cara menjalankan dengan safe (idempotent?).

---

## 17. Evaluasi SSO Cross-Service

### Pertanyaan Kunci: Apakah IAM Service bisa berfungsi sebagai SSO untuk semua services?

**Jawaban:** **Sebagian bisa bekerja, dengan beberapa masalah fundamental.**

### Yang Sudah Bisa Bekerja ✅

1. **Token sharing**: Finance service sudah bisa validasi JWT dari IAM menggunakan shared secret
2. **Permission model**: `service.module.entity.action` format mendukung multi-service (`iam.*`, `finance.*`, `hr.*`)
3. **RBAC**: Role-based access yang fleksibel mendukung multi-service scenarios
4. **Blacklist**: Finance membaca Redis blacklist IAM untuk revocation
5. **User context**: JWT menyertakan user_id, username, email, roles, permissions

### Yang Bermasalah ⚠

| Issue | Impact | Severity |
|-------|--------|----------|
| Symmetric JWT (HS256) | Secret harus di-share ke semua services | 🔴 High |
| No token introspection API | Services harus implement JWT validation sendiri | 🔴 High |
| Permissions di JWT payload | Token besar, cache stale (permission change tidak real-time) | 🟡 Medium |
| Redis coupling | Services harus bisa akses IAM Redis namespace | 🟡 Medium |
| No service identity | Tidak ada way untuk service-to-service auth (API keys tidak implemented) | 🟡 Medium |
| Single-device session | HR service dari iPad + Web akan conflict | 🟡 Medium |
| No session service scope filter | Semua token berlaku untuk semua services | 🟢 Low |

### Rekomendasi untuk True SSO

1. **Ganti HS256 → RS256**: IAM sign dengan private key, services verify dengan public key
2. **Implementasi Token Introspection gRPC endpoint** di IAM
3. **Implementasi API Keys** untuk service-to-service auth
4. **Implementasi multi-device session** (per service_name, bukan per user)
5. **JWKS endpoint** (JSON Web Key Set) untuk key distribution otomatis
6. **OAuth2/OIDC compliance** jika perlu support web apps dari domain lain

---

## 18. Ringkasan Temuan

### Statistik

| Kategori | Jumlah |
|----------|--------|
| Bug Kritis | 3 |
| Bug Medium | 3 |
| Gap Fungsionalitas | 10 |
| Inkonsistensi Pola | 4 |
| Technical Debt | 6 |
| Endpoint 501 Stub | 29 |
| Test Coverage Gap | 10 file |

### Prioritas Perbaikan

| Prioritas | Item |
|-----------|------|
| 🔴 P0 (Kritis) | BUG-003: Verifikasi password hashing di CreateUser |
| 🔴 P0 (Kritis) | BUG-001: Fix rate limiter per-method |
| 🔴 P0 (Kritis) | BUG-002: Fix session token hash inconsistency |
| 🔴 P1 (High) | GAP-002: Implementasi Token Introspection endpoint |
| 🔴 P1 (High) | Migrasi HS256 → RS256 untuk JWT |
| 🟡 P2 (Medium) | GAP-001: Implementasi API Keys |
| 🟡 P2 (Medium) | INCON-002: Refactor auth service menjadi handlers |
| 🟡 P2 (Medium) | BUG-004: Implementasi session cleanup scheduler |
| 🟡 P2 (Medium) | TOTP secret encryption at rest |
| 🟢 P3 (Low) | Export/Import bulk implementation (29 endpoints) |
| 🟢 P3 (Low) | Email templates, notifications |
| 🟢 P3 (Low) | CI/CD untuk IAM service |

---

*Laporan ini dibuat berdasarkan analisis mendalam dari seluruh codebase `goapps-backend` IAM service.*  
*Untuk detail rencana implementasi perbaikan, lihat dokumen `IAM_SERVICE_IMPROVEMENT_PLAN.md`.*
