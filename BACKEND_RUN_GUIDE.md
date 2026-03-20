# GoApps Backend — Panduan Menjalankan Project

Panduan lengkap untuk menjalankan seluruh backend microservices (IAM + Finance) di local development.

---

## Daftar Isi

1. [Arsitektur & Infrastruktur](#arsitektur--infrastruktur)
2. [Prerequisites](#prerequisites)
3. [Step 1: Jalankan Shared Infrastructure](#step-1-jalankan-shared-infrastructure)
4. [Step 2: Jalankan IAM Service](#step-2-jalankan-iam-service)
5. [Step 3: Jalankan Finance Service](#step-3-jalankan-finance-service)
6. [Step 4: Verifikasi Semua Berjalan](#step-4-verifikasi-semua-berjalan)
7. [Koneksi Antar Service](#koneksi-antar-service)
8. [Perintah Lengkap per Service](#perintah-lengkap-per-service)
9. [Port Map](#port-map)
10. [Troubleshooting](#troubleshooting)
11. [Stop & Cleanup](#stop--cleanup)

---

## Arsitektur & Infrastruktur

```
┌─────────────────────────────────────────────────────────────────┐
│                   docker-compose.yaml (backend root)            │
│                                                                 │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐              │
│  │ iam-postgres  │  │finance-postgres│  │  redis   │              │
│  │ :5435 → 5432  │  │ :5434 → 5432  │  │  :6379   │              │
│  │ iam_db        │  │ finance_db    │  │ DB0=Fin  │              │
│  │ user: iam     │  │ user: finance │  │ DB1=IAM  │              │
│  └──────────────┘  └──────────────┘  └──────────┘              │
│                                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐    │
│  │ mailpit  │  │  jaeger  │  │  minio   │  │  minio-init  │    │
│  │ SMTP:1025│  │ UI:16686 │  │ API:9000 │  │ (auto bucket)│    │
│  │ UI:8025  │  │ OTLP:4317│  │ UI:9001  │  │              │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────┘    │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────┐     ┌─────────────────────┐
│     IAM Service     │     │   Finance Service   │
│   gRPC  :50052      │     │   gRPC  :50051      │
│   HTTP  :8081       │     │   HTTP  :8080        │
│   (run di host)     │     │   (run di host)      │
└─────────────────────┘     └─────────────────────┘
```

**Penting**: Docker hanya untuk infrastruktur (PostgreSQL, Redis, dll). Service Go **dijalankan langsung di host** via `make run` atau `make dev`.

---

## Prerequisites

### Tools yang Dibutuhkan

| Tool | Versi | Install |
|------|-------|---------|
| Go | 1.24+ | https://go.dev/dl/ |
| Docker + Compose | v2+ | `docker compose version` |
| golang-migrate | v4.18.1 | `make install-tools` (di service dir) |
| golangci-lint | v1.62.2 | `make install-tools` |
| air | v1.52.3 | `make install-tools` (hot reload) |
| grpcurl | v1.9.1 | `make install-tools` |
| goimports | v0.28.0 | `make install-tools` |

### Install Semua Tools Sekaligus

```bash
# Dari salah satu service directory (tools sama untuk kedua service)
cd goapps-backend/services/iam
make install-tools

# Atau dari finance
cd goapps-backend/services/finance
make install-tools
```

---

## Step 1: Jalankan Shared Infrastructure

**Lokasi**: `goapps-backend/` (root backend directory)

```bash
cd goapps-backend

# Start semua container infrastruktur
docker compose up -d
```

Ini akan menjalankan:
- **iam-postgres** — PostgreSQL untuk IAM (`localhost:5435`)
- **finance-postgres** — PostgreSQL untuk Finance (`localhost:5434`)
- **redis** — Shared Redis (`localhost:6379`)
- **mailpit** — Email testing: SMTP di `:1025`, Web UI di http://localhost:8025
- **jaeger** — Distributed tracing: UI di http://localhost:16686
- **minio** — Object storage: API `:9000`, Console http://localhost:9001
- **minio-init** — Auto-create bucket `goapps-staging` lalu exit

### Verifikasi Infrastructure

```bash
# Cek semua container running
docker compose ps

# Cek health individual
docker exec goapps-iam-postgres pg_isready -U iam -d iam_db
docker exec goapps-finance-postgres pg_isready -U finance -d finance_db
docker exec goapps-redis redis-cli ping
```

**Expected output**:
- PostgreSQL: "accepting connections"
- Redis: "PONG"

### Akses Web UI

| Service | URL | Credentials |
|---------|-----|-------------|
| Mailpit | http://localhost:8025 | — |
| Jaeger | http://localhost:16686 | — |
| MinIO Console | http://localhost:9001 | minioadmin / minioadmin |

---

## Step 2: Jalankan IAM Service

**Lokasi**: `goapps-backend/services/iam/`

### 2a. Apply Migrations

```bash
cd goapps-backend/services/iam

make migrate-up
```

Ini menjalankan 7 migration files:
1. `000001_create_organization_tables` — company, division, department, section
2. `000002_create_user_tables` — users, user profiles
3. `000003_create_auth_tables` — login attempts, password reset, sessions
4. `000004_create_rbac_tables` — roles, permissions, role_permissions, user_roles
5. `000005_create_menu_tables` — menus, role_menus
6. `000006_create_audit_tables` — audit logs
7. `000007_create_recovery_codes_table` — 2FA recovery codes

**DATABASE_URL default**: `postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable`

Jika perlu override:
```bash
DATABASE_URL="postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable" make migrate-up
```

### 2b. Run Seeders

```bash
make seed
```

Seeder IAM akan membuat:
- Default company, division, department, section
- Admin user (`admin` / `admin123`)
- Default roles & permissions
- Menu structure
- Role-menu assignments

### 2c. Jalankan IAM Service

```bash
# Production-like
make run

# Atau dengan hot reload (development)
make dev
```

IAM service akan listen di:
- **gRPC**: `localhost:50052`
- **HTTP Gateway**: `localhost:8081` (Swagger UI di http://localhost:8081/swagger/)

### 2d. Verifikasi IAM

```bash
# List semua gRPC services
make grpc-list

# Health check
make grpc-health

# Test login
make grpc-login
# Atau manual:
grpcurl -plaintext -d '{"username": "admin", "password": "admin123", "device_info": "test"}' \
  localhost:50052 iam.v1.AuthService/Login
```

---

## Step 3: Jalankan Finance Service

**Lokasi**: `goapps-backend/services/finance/`

### 3a. Apply Migrations

```bash
cd goapps-backend/services/finance

make migrate-up
```

Ini menjalankan 2 migration files:
1. `000001_create_mst_uom` — Unit of Measure master table
2. `000002_create_audit_logs` — Audit log table

**DATABASE_URL default**: `postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable`

### 3b. Run Seeders

```bash
make seed
```

Seeder Finance akan membuat UOM data awal (kg, pcs, liter, dll).

### 3c. Jalankan Finance Service

```bash
# Production-like
make run

# Atau dengan hot reload (development)
make dev
```

Finance service akan listen di:
- **gRPC**: `localhost:50051`
- **HTTP Gateway**: `localhost:8080` (Swagger UI di http://localhost:8080/swagger/)

### 3d. Verifikasi Finance

```bash
# List semua gRPC services
make grpc-list

# Health check
make grpc-health

# List UOMs
make grpc-list-uoms
# Atau manual:
grpcurl -plaintext -d '{"page": 1, "page_size": 10}' \
  localhost:50051 finance.v1.UOMService/ListUOMs
```

---

## Step 4: Verifikasi Semua Berjalan

### Quick Health Check (semua sekaligus)

```bash
# Infrastructure
docker compose -f goapps-backend/docker-compose.yaml ps

# IAM Service
grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check

# Finance Service
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check

# Test login + get token
grpcurl -plaintext -d '{"username": "admin", "password": "admin123", "device_info": "test"}' \
  localhost:50052 iam.v1.AuthService/Login
```

### Test dengan Frontend

Jika frontend juga dijalankan (`npm run dev` di `goapps-frontend/`):
1. Buka http://localhost:3000
2. Login dengan `admin` / `admin123`
3. Frontend akan call API routes → gRPC ke backend services

---

## Koneksi Antar Service

### Shared Redis (DB Partitioning)

```
Redis :6379
├── DB 0 → Finance Service (UOM cache, general cache)
└── DB 1 → IAM Service (sessions, token blacklist, OTP, rate limit)
            ↑ Finance Service juga BACA DB 1 untuk validasi token blacklist
```

Ini artinya Finance service bisa memvalidasi bahwa JWT token belum di-revoke oleh IAM, tanpa perlu call IAM service langsung.

### Shared JWT Secret

Kedua service menggunakan JWT secret yang sama (`dev-access-secret-change-in-production`) agar Finance bisa memvalidasi token yang dikeluarkan IAM.

### Shared MinIO Bucket

```
goapps-staging/
├── iam/           → Avatar uploads, profile images
│   └── avatars/{user_id}/{filename}
└── finance/       → (future: document uploads, reports)
```

### Tracing (Jaeger)

Kedua service mengirim trace ke `localhost:4317` (OTLP gRPC). Lihat trace di http://localhost:16686.

### Email (Mailpit)

IAM service mengirim email (forgot password, OTP) ke Mailpit SMTP `:1025`. Lihat email di http://localhost:8025.

---

## Perintah Lengkap per Service

### IAM Service (`services/iam/`)

| Perintah | Deskripsi |
|----------|-----------|
| `make run` | Jalankan service |
| `make dev` | Hot reload dengan air |
| `make build` | Build binary ke `bin/iam-service` |
| `make test` | Semua test dengan race detection |
| `make test-unit` | Unit test saja (`./internal/...`) |
| `make test-integration` | Integration test (butuh DB) |
| `make test-ci-local` | Full CI test (start DB, migrate, test) |
| `make test-coverage` | Test dengan coverage report |
| `make lint` | golangci-lint |
| `make fmt` | go fmt + goimports |
| `make migrate-up` | Apply semua migration |
| `make migrate-down` | Rollback 1 migration terakhir |
| `make migrate-create NAME=xxx` | Buat migration baru |
| `make seed` | Jalankan seeder |
| `make grpc-list` | List gRPC services |
| `make grpc-health` | Health check |
| `make grpc-login` | Test login admin |
| `make proto-copy-swagger` | Copy swagger dari shared-proto |
| `make install-tools` | Install semua tools (pinned version) |

### Finance Service (`services/finance/`)

| Perintah | Deskripsi |
|----------|-----------|
| `make run` | Jalankan service |
| `make dev` | Hot reload dengan air |
| `make build` | Build binary ke `bin/finance-service` |
| `make test` | Semua test dengan race detection |
| `make test-unit` | Unit test saja |
| `make test-integration` | Integration test (butuh DB) |
| `make test-ci-local` | Full CI test |
| `make test-coverage` | Test dengan coverage report |
| `make lint` | golangci-lint |
| `make fmt` | go fmt + goimports |
| `make migrate-up` | Apply semua migration |
| `make migrate-down` | Rollback 1 migration terakhir |
| `make migrate-create NAME=xxx` | Buat migration baru |
| `make seed` | Jalankan seeder |
| `make grpc-list` | List gRPC services |
| `make grpc-health` | Health check |
| `make grpc-list-uoms` | Test list UOM |
| `make proto-copy-swagger` | Copy swagger dari shared-proto |
| `make install-tools` | Install semua tools (pinned version) |

### Root Makefile (`goapps-backend/`)

| Perintah | Deskripsi |
|----------|-----------|
| `make proto` | Generate proto code (dari shared-proto) |
| `make lint` | Lint semua services |
| `make test` | Test semua services |
| `make test-coverage` | Test + coverage report |
| `make finance-run` | Run finance service (dari root) |
| `make finance-build` | Build finance binary |
| `make finance-migrate` | Apply finance migrations |
| `make finance-seed` | Run finance seeder |
| `make clean` | Hapus build artifacts |

---

## Port Map

### Infrastructure (Docker)

| Service | Port | Protocol | Deskripsi |
|---------|------|----------|-----------|
| IAM PostgreSQL | 5435 | TCP | `iam_db` (user: iam, pass: iam123) |
| Finance PostgreSQL | 5434 | TCP | `finance_db` (user: finance, pass: finance123) |
| Redis | 6379 | TCP | Shared (no password) |
| Mailpit SMTP | 1025 | SMTP | Email testing |
| Mailpit Web | 8025 | HTTP | Email viewer |
| Jaeger UI | 16686 | HTTP | Trace viewer |
| Jaeger OTLP | 4317 | gRPC | Trace collector |
| Jaeger OTLP | 4318 | HTTP | Trace collector |
| Jaeger Collector | 14268 | HTTP | Legacy collector |
| MinIO API | 9000 | HTTP | S3-compatible API |
| MinIO Console | 9001 | HTTP | Web management |

### Go Services (Host)

| Service | gRPC Port | HTTP Port | Swagger |
|---------|-----------|-----------|---------|
| IAM | 50052 | 8081 | http://localhost:8081/swagger/ |
| Finance | 50051 | 8080 | http://localhost:8080/swagger/ |

---

## Troubleshooting

### "connection refused" saat migrate atau run

Pastikan docker sudah running:
```bash
cd goapps-backend
docker compose ps
# Jika belum running:
docker compose up -d
# Tunggu sampai healthy:
docker compose ps  # Cek kolom STATUS ada "(healthy)"
```

### "dirty database version"

Migration gagal di tengah jalan. Fix:
```bash
# Cek versi saat ini
migrate -path migrations/postgres -database "DATABASE_URL" version

# Force ke versi terakhir yang clean
migrate -path migrations/postgres -database "DATABASE_URL" force VERSION_NUMBER

# Lalu jalankan ulang
make migrate-up
```

### Port sudah dipakai

```bash
# Cek siapa yang pakai port
lsof -i :50051
lsof -i :50052
lsof -i :5434
lsof -i :5435

# Kill proses jika perlu
kill -9 <PID>
```

### Redis connection error

```bash
# Test koneksi Redis
redis-cli -h localhost -p 6379 ping
# Harus reply: PONG

# Cek DB yang dipakai
redis-cli -h localhost -p 6379 -n 1 keys '*'  # IAM keys
redis-cli -h localhost -p 6379 -n 0 keys '*'  # Finance keys
```

### MinIO bucket tidak ada

```bash
# Cek apakah minio-init sudah jalan
docker logs goapps-minio-init

# Buat manual jika perlu
docker exec -it goapps-minio mc alias set local http://localhost:9000 minioadmin minioadmin
docker exec -it goapps-minio mc mb --ignore-existing local/goapps-staging
```

### Seeder gagal / data sudah ada

Seeder biasanya idempotent (menggunakan `ON CONFLICT DO NOTHING` atau cek existence). Jika perlu reset:
```bash
# Drop dan recreate database
# IAM:
docker exec goapps-iam-postgres psql -U iam -d postgres -c "DROP DATABASE iam_db;"
docker exec goapps-iam-postgres psql -U iam -d postgres -c "CREATE DATABASE iam_db;"
cd services/iam && make migrate-up && make seed

# Finance:
docker exec goapps-finance-postgres psql -U finance -d postgres -c "DROP DATABASE finance_db;"
docker exec goapps-finance-postgres psql -U finance -d postgres -c "CREATE DATABASE finance_db;"
cd services/finance && make migrate-up && make seed
```

---

## Stop & Cleanup

### Stop Services

```bash
# Stop Go services: Ctrl+C di terminal masing-masing

# Stop infrastructure
cd goapps-backend
docker compose down
```

### Stop + Hapus Data (Full Reset)

```bash
cd goapps-backend
docker compose down -v  # -v hapus semua volumes (data hilang!)
```

### Restart Fresh (dari nol)

```bash
cd goapps-backend

# 1. Stop + hapus data
docker compose down -v

# 2. Start infrastructure
docker compose up -d

# 3. Tunggu healthy (~10 detik)
sleep 10

# 4. Migrate + Seed IAM
cd services/iam
make migrate-up
make seed

# 5. Migrate + Seed Finance
cd ../finance
make migrate-up
make seed

# 6. Jalankan services (buka 2 terminal)
# Terminal 1:
cd goapps-backend/services/iam && make run
# Terminal 2:
cd goapps-backend/services/finance && make run
```

---

## Urutan Start yang Benar (Ringkasan)

```
1. docker compose up -d          ← di goapps-backend/
2. (tunggu healthy)
3. make migrate-up               ← di services/iam/
4. make seed                     ← di services/iam/
5. make migrate-up               ← di services/finance/
6. make seed                     ← di services/finance/
7. make run (atau make dev)      ← di services/iam/     (terminal 1)
8. make run (atau make dev)      ← di services/finance/  (terminal 2)
```

IAM **harus dijalankan duluan** karena Finance membaca token blacklist dari Redis DB 1 yang dikelola IAM.

---

## IAM gRPC Services

Service yang tersedia di IAM (`localhost:50052`):

| Service | Deskripsi |
|---------|-----------|
| `iam.v1.AuthService` | Login, logout, refresh token, forgot/reset password, 2FA (TOTP) |
| `iam.v1.UserService` | CRUD users, profile, avatar upload |
| `iam.v1.RoleService` | CRUD roles |
| `iam.v1.PermissionService` | CRUD permissions |
| `iam.v1.MenuService` | Menu tree management |
| `iam.v1.SessionService` | Active session management |
| `iam.v1.AuditService` | Audit log viewer |
| `iam.v1.CompanyService` | Company management |
| `iam.v1.DivisionService` | Division management |
| `iam.v1.DepartmentService` | Department management |
| `iam.v1.SectionService` | Section management |

## Finance gRPC Services

Service yang tersedia di Finance (`localhost:50051`):

| Service | Deskripsi |
|---------|-----------|
| `finance.v1.UOMService` | Unit of Measure CRUD |

---

## Config Files

| Service | Config | Deskripsi |
|---------|--------|-----------|
| IAM | `services/iam/config.yaml` | DB, Redis, JWT, Email, TOTP, Security, Storage |
| Finance | `services/finance/config.yaml` | DB, Redis, JWT, CORS, Tracing, Rate Limit |
| Infra | `docker-compose.yaml` (root) | Semua container infrastructure |
