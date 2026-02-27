# IAM Service — Improvement & Implementation Plan

**Repo:** `goapps-backend`  
**Date:** 2026-02-27  
**Dokumen ini adalah:** Rencana perbaikan, penambahan, dan peningkatan IAM Service  
**Referensi:** `IAM_SERVICE_FINDINGS.md`

---

## Daftar Isi

1. [Executive Summary](#1-executive-summary)
2. [Roadmap Overview](#2-roadmap-overview)
3. [Phase 0 — Critical Bug Fixes (Sprint 1, ~3 hari)](#3-phase-0--critical-bug-fixes)
4. [Phase 1 — Security Hardening (Sprint 2-3, ~2 minggu)](#4-phase-1--security-hardening)
5. [Phase 2 — SSO Architecture (Sprint 4-5, ~2 minggu)](#5-phase-2--sso-architecture)
6. [Phase 3 — Feature Completion (Sprint 6-8, ~3 minggu)](#6-phase-3--feature-completion)
7. [Phase 4 — Code Quality & Consistency (Sprint 9-10, ~2 minggu)](#7-phase-4--code-quality--consistency)
8. [Phase 5 — Observability & Operations (Sprint 11, ~1 minggu)](#8-phase-5--observability--operations)
9. [Phase 6 — Testing & Documentation (Sprint 12, ~1 minggu)](#9-phase-6--testing--documentation)
10. [Detail Teknis Per Item](#10-detail-teknis-per-item)
11. [Migration Strategy](#11-migration-strategy)
12. [Dependency Additions](#12-dependency-additions)
13. [Estimasi Total](#13-estimasi-total)

---

## 1. Executive Summary

IAM service sudah memiliki fondasi arsitektur yang solid (DDD + Clean Architecture + gRPC). Namun terdapat beberapa masalah kritis yang perlu diselesaikan sebelum production-ready, terutama:

1. **3 bug kritis** harus diselesaikan segera (password hash, rate limiter, session hash)
2. **JWT algorithm** perlu bermigrasi dari HS256 ke RS256 untuk SSO yang benar
3. **Token Introspection** endpoint harus diimplementasi agar services lain tidak perlu share secret
4. **API Keys** untuk service-to-service auth
5. **29 endpoint stub** perlu diimplementasi untuk operasi bulk
6. **Inkonsistensi pola** application layer perlu di-refactor

Total estimasi: **~12 sprint (±3 bulan)** tergantung ukuran tim.

---

## 2. Roadmap Overview

```
Phase 0  │ P0 - Critical Bugs    │ Sprint 1        │ ~3 hari   │
Phase 1  │ P1 - Security         │ Sprint 2-3      │ ~2 minggu │
Phase 2  │ P1 - SSO Architecture │ Sprint 4-5      │ ~2 minggu │
Phase 3  │ P2 - Feature Complete │ Sprint 6-8      │ ~3 minggu │
Phase 4  │ P2 - Code Quality     │ Sprint 9-10     │ ~2 minggu │
Phase 5  │ P3 - Observability    │ Sprint 11       │ ~1 minggu │
Phase 6  │ P3 - Tests & Docs     │ Sprint 12       │ ~1 minggu │
─────────────────────────────────────────────────────────────────
Total                             12 sprint        ±3 bulan
```

---

## 3. Phase 0 — Critical Bug Fixes

> **Harus selesai sebelum ANY deployment ke staging/production.**

---

### TASK-001: Verifikasi dan Perbaiki CreateUser Password Hashing

**Priority:** 🔴 P0 Critical  
**Estimasi:** 2 jam  
**File:** `services/iam/internal/application/user/create_handler.go`

**Problem:**
`CreateCommand.PasswordHash` menerima `req.GetPassword()` (plain text dari proto request). Perlu dipastikan handler memanggil `h.passwordSvc.Hash()` sebelum meneruskan ke domain.

**Action Plan:**

```go
// SEBELUM (jika ini yang terjadi — SALAH):
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*CreateResult, error) {
    user, err := user.NewUser(cmd.Username, cmd.Email, cmd.PasswordHash, cmd.CreatedBy)
    // cmd.PasswordHash berisi plain text password!
}

// SESUDAH — BENAR:
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*CreateResult, error) {
    // Step 1: Validate password policy FIRST
    if err := h.passwordSvc.ValidatePolicy(cmd.Password); err != nil {
        return nil, fmt.Errorf("%w: %v", shared.ErrPasswordPolicy, err)
    }
    
    // Step 2: Hash password
    hashedPassword, err := h.passwordSvc.Hash(cmd.Password)
    if err != nil {
        return nil, fmt.Errorf("failed to hash password: %w", err)
    }
    
    // Step 3: Create domain entity with HASHED password
    user, err := user.NewUser(cmd.Username, cmd.Email, hashedPassword, cmd.CreatedBy)
    if err != nil {
        return nil, err
    }
    // ...
}
```

**Verifikasi:**
1. Buat user melalui API
2. Query DB langsung: `SELECT password_hash FROM mst_user WHERE username='...'`
3. Verifikasi format dimulai dengan `$argon2id$v=19$` bukan plain text
4. Jalankan test: `go test ./internal/application/user/... -v`

---

### TASK-002: Fix Rate Limiter Per-Method Interceptor

**Priority:** 🔴 P0 Critical  
**Estimasi:** 3 jam  
**File:** `services/iam/internal/delivery/grpc/rate_limiter.go` + `interceptors.go`

**Problem:**
`methodLimits` map didefinisikan tapi tidak pernah digunakan. Auth endpoints seharusnya dilindungi 5 rps.

**Action Plan:**

```go
// File: services/iam/internal/delivery/grpc/rate_limiter.go

type RateLimiter struct {
    globalLimiter *rate.Limiter
    methodLimits  map[string]*rate.Limiter // sudah ada, perlu dipakai
    mu            sync.RWMutex
}

// FIX: Tambahkan method untuk per-method + per-IP limiter
func (r *RateLimiter) AllowMethod(method string) bool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    // Check per-method limit first (more restrictive)
    if limiter, ok := r.methodLimits[method]; ok {
        if !limiter.Allow() {
            return false
        }
    }
    
    // Then check global limit
    return r.globalLimiter.Allow()
}

// FIX: Tambahkan Redis-based distributed per-IP rate limiter
func NewIPRateLimiter(redisClient *redis.Client, method string, ip string) bool {
    key := fmt.Sprintf("iam:ratelimit:%s:%s", method, ip)
    // Sliding window algorithm dengan Redis
    // ...
}
```

```go
// File: services/iam/internal/delivery/grpc/interceptors.go

func RateLimitInterceptor(limiter *RateLimiter) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler) (interface{}, error) {
        
        // FIX: Gunakan AllowMethod bukan Allow
        if !limiter.AllowMethod(info.FullMethod) {
            return nil, status.Error(codes.ResourceExhausted,
                "rate limit exceeded for method")
        }
        
        // FIX: Tambahkan per-IP check untuk auth endpoints
        if isAuthEndpoint(info.FullMethod) {
            ip := extractIPFromContext(ctx)
            if !limiter.AllowIP(ctx, info.FullMethod, ip) {
                return nil, status.Error(codes.ResourceExhausted,
                    "too many requests from this IP")
            }
        }
        
        return handler(ctx, req)
    }
}

func isAuthEndpoint(method string) bool {
    authEndpoints := map[string]bool{
        "/iam.v1.AuthService/Login":          true,
        "/iam.v1.AuthService/ForgotPassword": true,
        "/iam.v1.AuthService/VerifyResetOTP": true,
        "/iam.v1.AuthService/ResetPassword":  true,
    }
    return authEndpoints[method]
}
```

**Unit Test yang harus ditambah:**
```go
// File: services/iam/internal/delivery/grpc/rate_limiter_test.go
func TestRateLimiter_MethodSpecificLimit(t *testing.T)
func TestRateLimiter_GlobalLimit(t *testing.T)
func TestRateLimiter_AuthEndpointStrictLimit(t *testing.T)
```

---

### TASK-003: Fix Session Token Hash Inconsistency

**Priority:** 🔴 P0 Critical  
**Estimasi:** 4 jam  
**Files:** `services/iam/internal/application/auth/service.go`, `infrastructure/postgres/session_repository.go`, `domain/session/entity.go`

**Problem:**
Session dibuat dengan `hashToken(tokenPair.TokenID)` — hash dari UUID JTI, bukan full refresh token. Tapi `session.ValidateToken(refreshToken)` meng-hash full JWT string. `GetByRefreshToken(fullJWT)` akan gagal karena hash tidak cocok.

**Analysis:**
```
Saat Create Session:
  refreshTokenHash = SHA256(tokenPair.TokenID)  // JTI UUID: "abc-123-..."
  
Saat ValidateToken dipanggil (jika dipanggil):
  expectedHash = SHA256(fullJWT)                // full JWT: "eyJhbGci..."
  → MISMATCH!

Saat GetByTokenID dipanggil:
  WHERE refresh_token_hash = $1 ???             // menggunakan raw JTI, bukan hash?
```

**Decision:** Standarisasi ke hash dari JTI (lebih pendek dan efisien):

```go
// File: domain/session/entity.go

// UNIFIED: Selalu hash dari tokenID (JTI), bukan full token
func NewSession(userID uuid.UUID, tokenID string, deviceInfo, ip, service string, expiresAt time.Time) *Session {
    return &Session{
        ID:               uuid.New(),
        UserID:           userID,
        RefreshTokenHash: hashTokenID(tokenID),  // hash dari JTI
        DeviceInfo:       deviceInfo,
        IPAddress:        ip,
        ServiceName:      service,
        ExpiresAt:        expiresAt,
        CreatedAt:        time.Now(),
    }
}

func hashTokenID(tokenID string) string {
    hash := sha256.Sum256([]byte(tokenID))
    return hex.EncodeToString(hash[:])
}

// ValidateToken sekarang menerima tokenID (JTI), bukan full token
func (s *Session) ValidateByTokenID(tokenID string) error {
    expectedHash := hashTokenID(tokenID)
    if !strings.EqualFold(s.RefreshTokenHash, expectedHash) {
        return shared.ErrTokenRevoked
    }
    if s.IsExpired() {
        return shared.ErrTokenExpired
    }
    if s.revokedAt != nil {
        return shared.ErrTokenRevoked
    }
    return nil
}
```

```go
// File: infrastructure/postgres/session_repository.go

// GetByTokenID mencari berdasarkan HASH dari JTI (konsisten dengan Create)
func (r *SessionRepository) GetByTokenID(ctx context.Context, tokenID string) (*session.Session, error) {
    tokenHash := hashTokenID(tokenID)  // hash dulu
    query := `SELECT ... FROM user_sessions WHERE refresh_token_hash = $1`
    // ...
}
```

**Unit Tests:**
```go
func TestSession_HashConsistency(t *testing.T)
func TestSession_ValidateByTokenID(t *testing.T)
func TestSessionRepository_GetByTokenID_MatchesCreate(t *testing.T)
```

---

## 4. Phase 1 — Security Hardening

> **Selesaikan setelah Phase 0. Sebelum production launch.**

---

### TASK-004: TOTP Secret Encryption at Rest

**Priority:** 🔴 P1 High  
**Estimasi:** 1 hari  
**Files:** `infrastructure/postgres/user_repository.go`, `application/auth/service.go`, `config.yaml`

**Problem:**
`two_factor_secret` disimpan plaintext di column `mst_user`. Jika DB compromised, semua TOTP secrets terekspos.

**Action Plan:**

```go
// File: infrastructure/crypto/aes.go (NEW)

package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

type EncryptionService struct {
    key []byte // 32 bytes untuk AES-256
}

func NewEncryptionService(keyBase64 string) (*EncryptionService, error) {
    key, err := base64.StdEncoding.DecodeString(keyBase64)
    if err != nil {
        return nil, fmt.Errorf("invalid encryption key: %w", err)
    }
    if len(key) != 32 {
        return nil, fmt.Errorf("encryption key must be 32 bytes (AES-256), got %d", len(key))
    }
    return &EncryptionService{key: key}, nil
}

// Encrypt menggunakan AES-256-GCM (authenticated encryption)
func (e *EncryptionService) Encrypt(plaintext string) (string, error) {
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonce := make([]byte, gcm.NonceSize())
    if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *EncryptionService) Decrypt(encrypted string) (string, error) {
    data, err := base64.StdEncoding.DecodeString(encrypted)
    if err != nil {
        return "", fmt.Errorf("invalid encrypted data: %w", err)
    }
    
    block, err := aes.NewCipher(e.key)
    if err != nil {
        return "", err
    }
    
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    
    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }
    
    nonce, ciphertext := data[:nonceSize], data[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", fmt.Errorf("decryption failed: %w", err)
    }
    
    return string(plaintext), nil
}
```

```yaml
# config.yaml — tambahkan:
encryption:
  totp_secret_key: ""  # 32-byte key, base64-encoded. Generate: openssl rand -base64 32
```

```go
// File: infrastructure/postgres/user_repository.go
// Saat WRITE ke DB (Enable2FA):
encryptedSecret, err := r.encryptionSvc.Encrypt(totpSecret)
// Saat READ dari DB (Login, ValidateTOTP):
plainSecret, err := r.encryptionSvc.Decrypt(encryptedSecret)
```

**Migration:**
```sql
-- Tidak perlu schema change, tapi perlu data migration:
-- 1. Deploy kode baru dengan encryption service
-- 2. Jalankan migration script yang encrypt semua existing TOTP secrets
-- File: scripts/migrate_totp_secrets.go
```

---

### TASK-005: Migrasi HS256 ke RS256 (JWT Algorithm)

**Priority:** 🔴 P1 High (untuk true SSO)  
**Estimasi:** 3 hari  
**Files:** `infrastructure/jwt/service.go`, semua service yang validate token

**Problem:**
HS256 mensyaratkan semua services yang validate token memiliki `access_secret` yang sama (symmetric key). Ini:
1. Setiap service baru yang ditambahkan harus mendapat copy secret
2. Key rotation memerlukan restart semua services serentak
3. Jika satu service compromised, secret terekspos untuk semua

**Solution: RS256 (RSA-SHA256)**

```
IAM Service:
  - Memiliki: private key (untuk signing)
  - Expose: JWKS endpoint (public key untuk verify)

Finance Service, HR Service, dsb:
  - Fetch public key dari IAM JWKS endpoint
  - Cache public key (refresh periodically)
  - Tidak perlu memiliki private key sama sekali
```

**Action Plan:**

```go
// File: infrastructure/jwt/service.go — REFACTORED

package jwt

import (
    "crypto/rand"
    "crypto/rsa"
    "crypto/x509"
    "encoding/pem"
    "fmt"
    "os"
    
    "github.com/golang-jwt/jwt/v5"
)

type Service interface {
    GenerateTokenPair(claims AccessClaims) (*TokenPair, error)
    ParseAccessToken(tokenString string) (*AccessClaims, error)
    ParseRefreshToken(tokenString string) (*RefreshClaims, error)
    GetPublicKey() *rsa.PublicKey                    // NEW
    GetPublicKeyJWKS() ([]byte, error)               // NEW — untuk JWKS endpoint
}

type service struct {
    privateKey *rsa.PrivateKey  // hanya IAM yang punya ini
    publicKey  *rsa.PublicKey   // public untuk semua
    issuer     string
    kid        string           // Key ID untuk key rotation
    // Config
    accessTokenTTL  time.Duration
    refreshTokenTTL time.Duration
}

func NewService(cfg Config) (Service, error) {
    var privateKey *rsa.PrivateKey
    var err error
    
    // Load dari file (production) atau generate (development)
    if cfg.PrivateKeyPath != "" {
        privateKey, err = loadPrivateKey(cfg.PrivateKeyPath)
    } else if cfg.PrivateKeyPEM != "" {
        privateKey, err = parsePrivateKeyPEM(cfg.PrivateKeyPEM)
    } else {
        // Development: auto-generate (ephemeral, NOT for production)
        privateKey, err = rsa.GenerateKey(rand.Reader, 2048)
    }
    
    if err != nil {
        return nil, fmt.Errorf("failed to load JWT private key: %w", err)
    }
    
    return &service{
        privateKey:      privateKey,
        publicKey:       &privateKey.PublicKey,
        issuer:          cfg.Issuer,
        kid:             cfg.KeyID, // e.g., "iam-key-2026-01"
        accessTokenTTL:  cfg.AccessTokenTTL,
        refreshTokenTTL: cfg.RefreshTokenTTL,
    }, nil
}

func (s *service) GenerateTokenPair(claims AccessClaims) (*TokenPair, error) {
    // Sign dengan RS256 menggunakan private key
    accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
    accessToken.Header["kid"] = s.kid  // Key ID untuk JWKS lookup
    
    accessSigned, err := accessToken.SignedString(s.privateKey)
    // ...
}

func (s *service) ParseAccessToken(tokenString string) (*AccessClaims, error) {
    // Verify dengan public key saja (tidak perlu private key)
    token, err := jwt.ParseWithClaims(tokenString, &AccessClaims{},
        func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
            }
            return s.publicKey, nil
        },
    )
    // ...
}

// GetPublicKeyJWKS returns JSON Web Key Set format
func (s *service) GetPublicKeyJWKS() ([]byte, error) {
    // Standard JWKS format per RFC 7517
    jwks := map[string]interface{}{
        "keys": []map[string]interface{}{
            {
                "kty": "RSA",
                "use": "sig",
                "alg": "RS256",
                "kid": s.kid,
                "n":   base64URLEncode(s.publicKey.N.Bytes()),
                "e":   base64URLEncode(bigIntToBytes(s.publicKey.E)),
            },
        },
    }
    return json.Marshal(jwks)
}
```

```go
// File: delivery/httpdelivery/gateway.go — Tambahkan JWKS endpoint
mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
    jwks, err := jwtSvc.GetPublicKeyJWKS()
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Cache-Control", "public, max-age=3600")  // cache 1 jam
    w.Write(jwks)
})

// Juga: OpenID Connect Discovery endpoint
mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
    config := map[string]interface{}{
        "issuer":                  cfg.IAM.Issuer,
        "jwks_uri":               cfg.IAM.BaseURL + "/.well-known/jwks.json",
        "token_endpoint":         cfg.IAM.BaseURL + "/api/v1/iam/auth/login",
        "userinfo_endpoint":      cfg.IAM.BaseURL + "/api/v1/iam/auth/me",
        "response_types_supported": []string{"token"},
        "subject_types_supported": []string{"public"},
        "id_token_signing_alg_values_supported": []string{"RS256"},
    }
    json.NewEncoder(w).Encode(config)
})
```

```yaml
# config.yaml — Update jwt section:
jwt:
  private_key_path: "/run/secrets/jwt_private.pem"  # production: dari secret mount
  private_key_pem: ""                                # atau inline PEM (dev)
  key_id: "iam-key-2026-01"                          # untuk key rotation
  access_token_ttl: 15m
  refresh_token_ttl: 168h
  issuer: "goapps-iam"
```

**Finance Service Update:**
```go
// File: services/finance/internal/infrastructure/jwt/validator.go (NEW)

// Finance tidak butuh private key — hanya fetch public key dari IAM JWKS
type Validator struct {
    jwksURL     string
    publicKey   *rsa.PublicKey
    lastFetched time.Time
    cacheTTL    time.Duration
    mu          sync.RWMutex
}

func (v *Validator) ValidateToken(tokenString string) (*Claims, error) {
    // Auto-refresh public key setiap 1 jam
    if time.Since(v.lastFetched) > v.cacheTTL {
        if err := v.refreshPublicKey(); err != nil {
            // Fail gracefully — gunakan cached key jika masih ada
            if v.publicKey == nil {
                return nil, fmt.Errorf("unable to fetch public key: %w", err)
            }
        }
    }
    
    // Verify dengan public key
    token, err := jwt.ParseWithClaims(tokenString, &Claims{},
        func(token *jwt.Token) (interface{}, error) {
            return v.publicKey, nil
        },
    )
    // ...
}

func (v *Validator) refreshPublicKey() error {
    resp, err := http.Get(v.jwksURL)
    // Parse JWKS, extract RSA public key
    // ...
}
```

**Key Generation Commands (untuk Makefile):**
```makefile
# Tambahkan ke Makefile:
gen-jwt-keys:
	openssl genrsa -out jwt_private.pem 2048
	openssl rsa -in jwt_private.pem -pubout -out jwt_public.pem
	@echo "Keys generated. Store jwt_private.pem securely!"
	@echo "jwt_public.pem can be distributed freely."
```

---

### TASK-006: Implement Token Introspection gRPC Endpoint

**Priority:** 🔴 P1 High  
**Estimasi:** 2 hari  
**Files:** `gen/iam/v1/auth.proto` (update), `delivery/grpc/auth_handler.go`, `application/auth/service.go`

**Problem:**
Services lain tidak ada cara untuk memvalidasi token melalui IAM. Mereka harus validate sendiri (butuh secret) atau langsung baca Redis IAM (coupling).

**Action Plan:**

**1. Update Proto (auth.proto):**
```protobuf
// File: proto/iam/v1/auth.proto (update)

service AuthService {
    // ... existing methods ...
    
    // ValidateToken: Token introspection untuk service-to-service
    // Dipanggil oleh services lain untuk memvalidasi token tanpa butuh JWT secret
    rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse) {
        option (google.api.http) = {
            post: "/api/v1/iam/auth/validate"
            body: "*"
        };
    }
}

message ValidateTokenRequest {
    string token = 1;            // access token yang akan divalidasi
    string service_name = 2;     // service mana yang request (untuk audit)
}

message ValidateTokenResponse {
    bool active = 1;             // true jika token valid dan aktif
    string user_id = 2;
    string username = 3;
    string email = 4;
    repeated string roles = 5;
    repeated string permissions = 6;
    int64 expires_at = 7;        // Unix timestamp
    string token_id = 8;         // JTI untuk reference
    // Error info jika active=false:
    string reason = 9;           // "expired", "revoked", "invalid"
}
```

**2. Application Layer:**
```go
// File: services/iam/internal/application/auth/validate_token_handler.go (NEW)

package auth

type ValidateTokenHandler struct {
    jwtSvc  jwt.Service
    cache   session.CacheRepository
    userRepo user.Repository
}

type ValidateTokenCommand struct {
    Token       string
    ServiceName string
    RequiredPermission string  // optional: cek permission spesifik
}

type ValidateTokenResult struct {
    Active      bool
    UserID      uuid.UUID
    Username    string
    Email       string
    Roles       []string
    Permissions []string
    ExpiresAt   time.Time
    TokenID     string
    Reason      string  // jika !Active
}

func (h *ValidateTokenHandler) Handle(ctx context.Context, cmd ValidateTokenCommand) (*ValidateTokenResult, error) {
    // 1. Parse token
    claims, err := h.jwtSvc.ParseAccessToken(cmd.Token)
    if err != nil {
        return &ValidateTokenResult{Active: false, Reason: "invalid"}, nil
    }
    
    // 2. Check expiry
    if claims.ExpiresAt.Time.Before(time.Now()) {
        return &ValidateTokenResult{Active: false, Reason: "expired"}, nil
    }
    
    // 3. Check blacklist
    blacklisted, err := h.cache.IsTokenBlacklisted(ctx, claims.ID)
    if err == nil && blacklisted {
        return &ValidateTokenResult{Active: false, Reason: "revoked"}, nil
    }
    
    // 4. Optional: Check specific permission
    if cmd.RequiredPermission != "" {
        if !hasPermission(claims.Permissions, cmd.RequiredPermission) {
            return &ValidateTokenResult{Active: false, Reason: "insufficient_permissions"}, nil
        }
    }
    
    return &ValidateTokenResult{
        Active:      true,
        UserID:      claims.UserID,
        Username:    claims.Username,
        Email:       claims.Email,
        Roles:       claims.Roles,
        Permissions: claims.Permissions,
        ExpiresAt:   claims.ExpiresAt.Time,
        TokenID:     claims.ID,
    }, nil
}
```

**3. gRPC Handler:**
```go
// File: delivery/grpc/auth_handler.go — tambahkan:

func (h *AuthHandler) ValidateToken(ctx context.Context, req *iamv1.ValidateTokenRequest) (*iamv1.ValidateTokenResponse, error) {
    // Ini endpoint PUBLIC (no auth required, dipanggil oleh services lain)
    // tapi rate-limited untuk mencegah abuse
    
    result, err := h.validateTokenHandler.Handle(ctx, auth.ValidateTokenCommand{
        Token:       req.GetToken(),
        ServiceName: req.GetServiceName(),
    })
    if err != nil {
        return nil, err
    }
    
    return &iamv1.ValidateTokenResponse{
        Active:      result.Active,
        UserId:      result.UserID.String(),
        Username:    result.Username,
        Email:       result.Email,
        Roles:       result.Roles,
        Permissions: result.Permissions,
        ExpiresAt:   result.ExpiresAt.Unix(),
        TokenId:     result.TokenID,
        Reason:      result.Reason,
    }, nil
}
```

**4. Tambahkan ke publicMethods:**
```go
var publicMethods = map[string]bool{
    // ... existing ...
    "/iam.v1.AuthService/ValidateToken": true,  // public, rate-limited
}
```

**5. Update Finance Service:**
```go
// File: services/finance/internal/delivery/grpc/auth_interceptor.go — UPDATE

// SEBELUM: Finance validate JWT sendiri + baca Redis IAM
// SESUDAH: Finance call IAM ValidateToken gRPC

type AuthInterceptor struct {
    iamClient iamv1.AuthServiceClient  // gRPC client ke IAM
    cache     *redis.Client            // local cache untuk hasil introspection
}

func (i *AuthInterceptor) validate(ctx context.Context, token string) (*TokenClaims, error) {
    // Check local cache dulu (cache 5 menit untuk performance)
    cacheKey := "auth:token:" + hashToken(token)
    if cached, ok := i.getFromCache(cacheKey); ok {
        return cached, nil
    }
    
    // Call IAM introspection
    resp, err := i.iamClient.ValidateToken(ctx, &iamv1.ValidateTokenRequest{
        Token:       token,
        ServiceName: "finance",
    })
    if err != nil {
        // Fail-open jika IAM down (dengan log warning)
        // Opsi: fail-closed untuk security-critical services
        return nil, fmt.Errorf("unable to validate token: %w", err)
    }
    
    if !resp.Active {
        return nil, shared.ErrInvalidToken
    }
    
    claims := &TokenClaims{...}
    i.setCache(cacheKey, claims, 5*time.Minute)
    return claims, nil
}
```

---

### TASK-007: Implementasi API Key Authentication

**Priority:** 🟡 P1 Medium  
**Estimasi:** 3 hari  
**Files:** New files untuk API key domain, repo, handler

**Problem:**
Schema `api_keys` sudah ada di DB tapi belum ada implementasi. Service-to-service auth (HR service → IAM) tidak ada mechanism yang proper.

**Action Plan:**

**1. Domain Entity:**
```go
// File: domain/apikey/entity.go (NEW)

package apikey

type APIKey struct {
    id                  uuid.UUID
    userID              uuid.UUID
    keyName             string
    keyHash             string      // SHA256 of actual key
    keyPrefix           string      // first 16 chars untuk display
    allowedIPs          []string    // CIDR notation supported
    allowedScopes       []string    // permission codes
    serviceName         string
    rateLimitPerMinute  int
    expiresAt           *time.Time
    lastUsedAt          *time.Time
    isActive            bool
    revokedAt           *time.Time
    createdAt           time.Time
    createdBy           uuid.UUID
}

// Generate new API key
func NewAPIKey(userID uuid.UUID, name, service string, scopes []string, createdBy uuid.UUID) (*APIKey, string, error) {
    // Generate 32-byte random key
    rawKey := make([]byte, 32)
    if _, err := rand.Read(rawKey); err != nil {
        return nil, "", err
    }
    
    // Encode sebagai "iam_" prefix + base64url (identifiable format)
    keyString := "iam_" + base64.RawURLEncoding.EncodeToString(rawKey)
    
    // Hash untuk storage (SHA256)
    hash := sha256.Sum256([]byte(keyString))
    keyHash := hex.EncodeToString(hash[:])
    
    return &APIKey{
        id:          uuid.New(),
        userID:      userID,
        keyName:     name,
        keyHash:     keyHash,
        keyPrefix:   keyString[:16],  // "iam_XXXXXXXX" untuk display
        allowedScopes: scopes,
        serviceName:   service,
        isActive:    true,
        createdAt:   time.Now(),
        createdBy:   createdBy,
    }, keyString, nil  // keyString dikembalikan SEKALI SAJA, tidak disimpan plaintext
}

func (k *APIKey) VerifyKey(inputKey string) bool {
    hash := sha256.Sum256([]byte(inputKey))
    expected := hex.EncodeToString(hash[:])
    return subtle.ConstantTimeCompare([]byte(k.keyHash), []byte(expected)) == 1
}

func (k *APIKey) IsAllowedIP(ip string) bool {
    if len(k.allowedIPs) == 0 {
        return true  // tidak ada IP restriction
    }
    // Check CIDR match
    for _, allowedIP := range k.allowedIPs {
        if matchCIDR(ip, allowedIP) {
            return true
        }
    }
    return false
}

func (k *APIKey) HasScope(scope string) bool {
    for _, s := range k.allowedScopes {
        if s == scope || s == "*" {
            return true
        }
    }
    return false
}
```

**2. Application Handlers:**
```go
// File: application/apikey/create_handler.go (NEW)
// File: application/apikey/revoke_handler.go (NEW)
// File: application/apikey/list_handler.go (NEW)
// File: application/apikey/rotate_handler.go (NEW) — generate key baru, invalidate lama
```

**3. Auth Interceptor Update:**
```go
// File: delivery/grpc/auth_interceptor.go — Update untuk support API Key

func (i *AuthInterceptor) authenticate(ctx context.Context, md metadata.MD) (*AuthContext, error) {
    // 1. Coba Bearer token (user JWT)
    if bearer := extractBearer(md); bearer != "" {
        return i.authenticateJWT(ctx, bearer)
    }
    
    // 2. Coba API Key (X-API-Key header)
    if apiKey := extractAPIKey(md); apiKey != "" {
        return i.authenticateAPIKey(ctx, apiKey)
    }
    
    return nil, shared.ErrUnauthorized
}

func (i *AuthInterceptor) authenticateAPIKey(ctx context.Context, rawKey string) (*AuthContext, error) {
    // 1. Hash key
    keyHash := hashKey(rawKey)
    
    // 2. Lookup di DB
    apiKey, err := i.apiKeyRepo.GetByHash(ctx, keyHash)
    if err != nil {
        return nil, shared.ErrInvalidToken
    }
    
    // 3. Verify key dan state
    if !apiKey.IsActive() {
        return nil, shared.ErrTokenRevoked
    }
    if !apiKey.VerifyKey(rawKey) {
        return nil, shared.ErrInvalidToken
    }
    
    // 4. IP check
    ip := extractIPFromContext(ctx)
    if !apiKey.IsAllowedIP(ip) {
        return nil, shared.ErrPermissionDenied
    }
    
    // 5. Rate limit per API key
    if err := i.checkAPIKeyRateLimit(ctx, apiKey.ID()); err != nil {
        return nil, err
    }
    
    // 6. Update last_used_at (async)
    go i.apiKeyRepo.UpdateLastUsed(context.Background(), apiKey.ID())
    
    // 7. Return auth context dengan scopes dari API key
    return &AuthContext{
        UserID:      apiKey.UserID(),
        IsAPIKey:    true,
        APIKeyID:    apiKey.ID(),
        Permissions: apiKey.Scopes(),
    }, nil
}
```

---

## 5. Phase 2 — SSO Architecture

> **Implementasi fitur-fitur untuk SSO yang benar dan scalable.**

---

### TASK-008: Multi-Device Session Support

**Priority:** 🟡 P2 Medium  
**Estimasi:** 2 hari  
**Files:** `migrations/postgres/000003_...`, `domain/session/`, `application/auth/service.go`

**Problem:**
Single-device policy (partial unique index) mencegah user login dari web browser DAN mobile app secara bersamaan. Untuk HR/Finance app yang perlu akses dari berbagai device, ini sangat membatasi.

**Action Plan:**

```sql
-- Migration: 000008_update_session_multi_device.up.sql

-- Drop the single-device constraint
DROP INDEX IF EXISTS idx_user_active_session;

-- Add new index supporting per-device or per-service sessions
-- User boleh punya 1 active session per service_name
CREATE UNIQUE INDEX idx_user_service_active_session
    ON user_sessions(user_id, service_name)
    WHERE revoked_at IS NULL;

-- Tambahkan device_type column
ALTER TABLE user_sessions ADD COLUMN device_type VARCHAR(50);
-- Values: "web", "mobile", "desktop", "service" (untuk API keys)
```

```go
// domain/session/entity.go — Update NewSession
func NewSession(userID uuid.UUID, tokenID, deviceInfo, ip, service, deviceType string, expiresAt time.Time) *Session {
    return &Session{
        // ...
        DeviceType:  deviceType,
        ServiceName: service,
    }
}
```

```go
// application/auth/service.go — Login flow update
// SEBELUM: revoke ALL existing active sessions
// SESUDAH: revoke only sessions for same service_name

func (s *Service) revokeExistingSessions(ctx context.Context, userID uuid.UUID, serviceName string) error {
    return s.sessionRepo.RevokeByUserIDAndService(ctx, userID, serviceName)
}
```

**Config untuk max sessions:**
```yaml
session:
  max_per_service: 1      # max active sessions per service per user
  max_total: 5            # max total active sessions per user
  cleanup_interval: 24h   # auto cleanup expired sessions
```

---

### TASK-009: Implementasi Session Cleanup Scheduler

**Priority:** 🟡 P2 Medium  
**Estimasi:** 4 jam  
**Files:** `cmd/server/main.go`, `infrastructure/scheduler/` (NEW)

**Problem:**
`session.CleanupExpired()` didefinisikan tapi tidak pernah dipanggil. Expired sessions terus menumpuk di DB.

**Action Plan:**

```go
// File: infrastructure/scheduler/session_cleanup.go (NEW)

package scheduler

import (
    "context"
    "time"
    
    "github.com/rs/zerolog/log"
)

type SessionCleanupJob struct {
    sessionRepo session.Repository
    interval    time.Duration
}

func NewSessionCleanupJob(repo session.Repository, interval time.Duration) *SessionCleanupJob {
    return &SessionCleanupJob{
        sessionRepo: repo,
        interval:    interval,
    }
}

func (j *SessionCleanupJob) Start(ctx context.Context) {
    ticker := time.NewTicker(j.interval)
    defer ticker.Stop()
    
    log.Info().Dur("interval", j.interval).Msg("session cleanup scheduler started")
    
    for {
        select {
        case <-ctx.Done():
            log.Info().Msg("session cleanup scheduler stopped")
            return
        case <-ticker.C:
            j.run(ctx)
        }
    }
}

func (j *SessionCleanupJob) run(ctx context.Context) {
    start := time.Now()
    
    if err := j.sessionRepo.CleanupExpired(ctx); err != nil {
        log.Error().Err(err).Msg("failed to cleanup expired sessions")
        return
    }
    
    log.Debug().Dur("duration", time.Since(start)).Msg("expired sessions cleaned up")
}
```

```go
// File: cmd/server/main.go — tambahkan:

// Start session cleanup scheduler
cleanupJob := scheduler.NewSessionCleanupJob(sessionRepo, 24*time.Hour)
go cleanupJob.Start(ctx)
```

---

### TASK-010: Password Reset Token Persistence ke DB

**Priority:** 🟡 P2 Medium  
**Estimasi:** 1 hari  
**Files:** `infrastructure/postgres/password_reset_repository.go` (NEW), `application/auth/service.go`

**Problem:**
`password_reset_tokens` table dibuat tapi tidak digunakan. OTP flow hanya pakai Redis, sehingga OTP hilang jika Redis restart.

**Action Plan:**

```go
// File: domain/passwordreset/entity.go (NEW)

type ResetToken struct {
    id       uuid.UUID
    userID   uuid.UUID
    tokenHash string
    otpCode   string   // 6-digit (store hashed in DB)
    expiresAt time.Time
    isUsed    bool
    createdAt time.Time
}

// Repository interface
type Repository interface {
    Create(ctx context.Context, token *ResetToken) error
    GetByTokenHash(ctx context.Context, tokenHash string) (*ResetToken, error)
    GetByUserIDAndOTP(ctx context.Context, userID uuid.UUID, otpHash string) (*ResetToken, error)
    MarkUsed(ctx context.Context, id uuid.UUID) error
    CleanupExpired(ctx context.Context) error
}
```

**Design:**
- OTP 6-digit → store hashed (SHA256) di DB, juga simpan di Redis untuk fast lookup
- Reset token → random 32-byte hex, store SHA256 di DB
- TTL enforced oleh `expires_at` column (not just Redis TTL)
- Cleanup expired tokens via scheduled job

---

### TASK-011: Password Expiry Policy

**Priority:** 🟡 P2 Medium  
**Estimasi:** 1 hari  
**Files:** `application/auth/service.go`, `infrastructure/config/config.go`, `domain/user/entity.go`

**Action Plan:**

```yaml
# config.yaml — tambahkan:
security:
  password_expiry_days: 90    # 0 = tidak ada expiry
  password_expiry_warning_days: 7  # kirim warning 7 hari sebelum
```

```go
// domain/user/entity.go — tambahkan:
func (u *User) IsPasswordExpired(expiryDays int) bool {
    if expiryDays <= 0 {
        return false
    }
    if u.passwordChangedAt == nil {
        return true  // never changed = expired
    }
    return time.Since(*u.passwordChangedAt) > time.Duration(expiryDays)*24*time.Hour
}

func (u *User) DaysUntilPasswordExpiry(expiryDays int) int {
    if expiryDays <= 0 {
        return -1  // tidak ada expiry
    }
    if u.passwordChangedAt == nil {
        return 0
    }
    expiry := u.passwordChangedAt.Add(time.Duration(expiryDays) * 24 * time.Hour)
    remaining := time.Until(expiry)
    if remaining < 0 {
        return 0
    }
    return int(remaining.Hours() / 24)
}
```

```go
// application/auth/service.go — tambahkan di Login flow:
// Setelah login sukses, sebelum generate token:

if cfg.Security.PasswordExpiryDays > 0 {
    daysLeft := user.DaysUntilPasswordExpiry(cfg.Security.PasswordExpiryDays)
    if daysLeft == 0 {
        // Password expired — force change
        return nil, fmt.Errorf("%w: password has expired, please change it", shared.ErrPasswordExpired)
    }
    // Warning via response metadata
    if daysLeft <= cfg.Security.PasswordExpiryWarningDays {
        // Sertakan warning di response
        result.PasswordExpiresInDays = daysLeft
    }
}
```

---

### TASK-012: Bcrypt Auto-Migration ke Argon2id

**Priority:** 🟢 P2 Low  
**Estimasi:** 2 jam  
**Files:** `application/auth/service.go`

**Problem:**
User dengan bcrypt hash tidak pernah di-upgrade ke Argon2id setelah login sukses.

**Action Plan:**

```go
// application/auth/service.go — di Login flow setelah password verify sukses:

// Cek apakah password hash perlu di-upgrade
if s.passwordSvc.NeedsUpgrade(user.PasswordHash()) {
    newHash, err := s.passwordSvc.Hash(cmd.Password)
    if err == nil {
        // Upgrade hash silently
        user.UpdatePassword(newHash)
        if updateErr := s.userRepo.Update(ctx, user); updateErr != nil {
            // Log warning tapi jangan gagalkan login
            log.Warn().Err(updateErr).Str("user_id", user.ID().String()).
                Msg("failed to upgrade password hash")
        }
    }
}
```

```go
// infrastructure/password/service.go — tambahkan:
func (s *Service) NeedsUpgrade(hash string) bool {
    // bcrypt prefix = perlu upgrade ke argon2id
    return strings.HasPrefix(hash, "$2a$") ||
           strings.HasPrefix(hash, "$2b$") ||
           strings.HasPrefix(hash, "$2y$")
}
```

---

## 6. Phase 3 — Feature Completion

> **Implementasi fitur-fitur yang saat ini 501 Not Implemented.**

---

### TASK-013: Implementasi Export/Import — User, Role, Permission

**Priority:** 🟡 P2 Medium  
**Estimasi:** 5 hari  
**Files:** Multiple files di application/ dan delivery/

**Note:** Finance service sudah punya implementasi Excel export/import lengkap sebagai referensi (`services/finance/internal/application/uom/export_handler.go`, `import_handler.go`). Gunakan pola yang sama untuk IAM.

**Excel Format untuk User Export:**

```
Sheet "Users":
Columns:
  A: Username
  B: Email
  C: Full Name
  D: Employee Code
  E: Position
  F: Section
  G: Department
  H: Division
  I: Company
  J: Status (Active/Inactive)
  K: 2FA Enabled (Yes/No)
  L: Last Login
  M: Created At

Sheet "User Roles":
  A: Username
  B: Role Code
  C: Role Name
```

**Action Plan:**

```go
// File: application/user/export_handler.go (NEW)

package user

import "github.com/xuri/excelize/v2"

type ExportHandler struct {
    userRepo user.Repository
    orgRepo  organization.Repository
}

type ExportCommand struct {
    Filter   user.Filter
    Format   string          // "excel" atau "csv"
    RequestedBy uuid.UUID
}

type ExportResult struct {
    Data        []byte
    ContentType string
    Filename    string
}

func (h *ExportHandler) Handle(ctx context.Context, cmd ExportCommand) (*ExportResult, error) {
    // 1. Fetch all users (tanpa pagination)
    users, _, err := h.userRepo.ListWithDetails(ctx, user.Filter{
        // Apply filter tapi MaxPerPage = unlimited
        Limit: 10000,
    })
    if err != nil {
        return nil, err
    }
    
    // 2. Generate Excel
    f := excelize.NewFile()
    defer f.Close()
    
    sheet := "Users"
    f.SetSheetName("Sheet1", sheet)
    
    // Headers
    headers := []string{"Username", "Email", "Full Name", "Employee Code", "Position",
                         "Section", "Department", "Division", "Company", "Status",
                         "2FA Enabled", "Last Login", "Created At"}
    for i, h := range headers {
        cell, _ := excelize.CoordinatesToCellName(i+1, 1)
        f.SetCellValue(sheet, cell, h)
    }
    
    // Style header
    style, _ := f.NewStyle(&excelize.Style{
        Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
        Fill: excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
    })
    f.SetCellStyle(sheet, "A1", "M1", style)
    
    // Data rows
    for row, u := range users {
        rowNum := row + 2
        f.SetCellValue(sheet, fmt.Sprintf("A%d", rowNum), u.User.Username())
        f.SetCellValue(sheet, fmt.Sprintf("B%d", rowNum), u.User.Email())
        // ...
    }
    
    // Auto-fit columns
    for i := range headers {
        col, _ := excelize.ColumnNumberToName(i + 1)
        f.SetColWidth(sheet, col, col, 20)
    }
    
    buf, err := f.WriteToBuffer()
    if err != nil {
        return nil, err
    }
    
    return &ExportResult{
        Data:        buf.Bytes(),
        ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        Filename:    fmt.Sprintf("users_%s.xlsx", time.Now().Format("20060102_150405")),
    }, nil
}
```

```go
// File: application/user/import_handler.go (NEW)

type ImportHandler struct {
    createHandler  *CreateHandler
    orgRepo        organization.Repository
    roleRepo       role.Repository
}

type ImportCommand struct {
    FileData    []byte
    Format      string
    RequestedBy uuid.UUID
    DryRun      bool         // validasi saja tanpa simpan
}

type ImportResult struct {
    TotalRows    int
    Imported     int
    Failed       int
    Errors       []ImportError
}

type ImportError struct {
    Row     int
    Field   string
    Message string
}

func (h *ImportHandler) Handle(ctx context.Context, cmd ImportCommand) (*ImportResult, error) {
    f, err := excelize.OpenReader(bytes.NewReader(cmd.FileData))
    if err != nil {
        return nil, fmt.Errorf("invalid Excel file: %w", err)
    }
    
    rows, err := f.GetRows("Users")
    if err != nil {
        return nil, fmt.Errorf("sheet 'Users' not found: %w", err)
    }
    
    result := &ImportResult{TotalRows: len(rows) - 1}  // skip header
    
    for i, row := range rows[1:] {  // skip header row
        rowNum := i + 2
        
        if err := h.validateAndImportRow(ctx, rowNum, row, cmd, result); err != nil {
            result.Failed++
            result.Errors = append(result.Errors, ImportError{Row: rowNum, Message: err.Error()})
        }
    }
    
    return result, nil
}
```

---

### TASK-014: Implementasi Email Templates

**Priority:** 🟡 P2 Medium  
**Estimasi:** 2 hari  
**Files:** `infrastructure/email/service.go`, `infrastructure/email/templates/` (NEW)

**Problem:**
Email service hanya kirim plain text. Untuk professional SSO system, HTML email diperlukan.

**Action Plan:**

```go
// File: infrastructure/email/templates/otp_reset.html (NEW)
// File: infrastructure/email/templates/welcome.html (NEW)
// File: infrastructure/email/templates/account_locked.html (NEW)
// File: infrastructure/email/templates/2fa_enabled.html (NEW)
// File: infrastructure/email/templates/password_expiry_warning.html (NEW)
// File: infrastructure/email/templates/suspicious_login.html (NEW)

// File: infrastructure/email/service.go — REFACTORED

type EmailData struct {
    To        string
    Subject   string
    Template  string
    Variables map[string]interface{}
}

func (s *Service) sendTemplated(data EmailData) error {
    // Load template dari embedded FS
    tmpl, err := template.ParseFS(emailTemplates, "templates/"+data.Template)
    if err != nil {
        return fmt.Errorf("template not found: %w", err)
    }
    
    var htmlBuf, textBuf bytes.Buffer
    if err := tmpl.ExecuteTemplate(&htmlBuf, "html", data.Variables); err != nil {
        return err
    }
    if err := tmpl.ExecuteTemplate(&textBuf, "text", data.Variables); err != nil {
        return err
    }
    
    // Send multipart email (text + HTML)
    return s.sendMultipart(data.To, data.Subject, textBuf.String(), htmlBuf.String())
}
```

**New email notifications to implement:**
1. `SendWelcome(email, name, temporaryPassword)` — saat user baru dibuat
2. `SendAccountLocked(email, name, unlocksAt)` — saat account di-lock
3. `SendPasswordExpiryWarning(email, name, daysLeft)` — sebelum password expired
4. `SendSuspiciousLogin(email, name, ip, location, time)` — login dari IP/device baru
5. Update `SendOTP` dengan HTML template

---

### TASK-015: Implementasi Export/Import — Menu, Organization

**Priority:** 🟢 P3 Low  
**Estimasi:** 3 hari

Sama seperti TASK-013 untuk Menu dan Organization entities.

---

### TASK-016: Implementasi Export — Audit Logs

**Priority:** 🟢 P3 Low  
**Estimasi:** 1 hari

Export audit logs ke Excel/CSV dengan filter tanggal, event type, user, dsb.

---

## 7. Phase 4 — Code Quality & Consistency

> **Refactoring untuk konsistensi dan maintainability.**

---

### TASK-017: Refactor Auth Service menjadi Command Handlers

**Priority:** 🟡 P2 Medium  
**Estimasi:** 3 hari  
**Files:** `application/auth/service.go` → multiple handler files

**Problem:**
`auth/service.go` adalah God Object dengan 11 methods dan 10+ dependencies.

**Action Plan:**

```
SEBELUM:
application/auth/service.go (1 file, ~500 baris, 11 methods)

SESUDAH:
application/auth/
  ├── login_handler.go        (Login)
  ├── logout_handler.go       (Logout)
  ├── refresh_handler.go      (RefreshToken)
  ├── forgot_password_handler.go   (ForgotPassword + VerifyResetOTP + ResetPassword)
  ├── update_password_handler.go   (UpdatePassword)
  ├── enable_2fa_handler.go   (Enable2FA + Verify2FA)
  ├── disable_2fa_handler.go  (Disable2FA)
  ├── get_current_user_handler.go  (GetCurrentUser)
  └── validate_token_handler.go    (ValidateToken — NEW dari TASK-006)
```

```go
// Contoh struktur yang dipisah:

// login_handler.go
type LoginHandler struct {
    userRepo    user.Repository
    sessionRepo session.Repository
    cacheRepo   session.CacheRepository
    jwtSvc      jwt.Service
    passSvc     password.Service
    totpSvc     totp.Service
    auditRepo   audit.Repository
}

// refresh_handler.go
type RefreshTokenHandler struct {
    sessionRepo session.Repository
    cacheRepo   session.CacheRepository
    jwtSvc      jwt.Service
    auditRepo   audit.Repository
}

// enable_2fa_handler.go
type Enable2FAHandler struct {
    userRepo  user.Repository
    totpSvc   totp.Service
    cacheRepo session.CacheRepository
    emailSvc  email.Service
}
```

---

### TASK-018: Refactor Menu & Organization ke Handler Pattern

**Priority:** 🟡 P2 Medium  
**Estimasi:** 2 hari  
**Files:** `application/menu/service.go`, `application/organization/service.go`

Ikuti pola yang sudah ada di `application/user/` dan `application/role/`.

```
application/menu/
  ├── create_handler.go
  ├── get_handler.go
  ├── list_handler.go
  ├── update_handler.go
  ├── delete_handler.go
  ├── reorder_handler.go
  └── assign_permissions_handler.go

application/organization/
  ├── create_company_handler.go
  ├── list_company_handler.go
  ├── ... (per entity, per operation)
```

---

### TASK-019: Implementasi Domain Events (Opsional tapi Recommended)

**Priority:** 🟢 P3 Low  
**Estimasi:** 3 hari

```go
// File: domain/shared/events.go (NEW)

type DomainEvent interface {
    EventType() string
    AggregateID() uuid.UUID
    OccurredAt() time.Time
}

type UserCreatedEvent struct {
    UserID   uuid.UUID
    Username string
    Email    string
    ts       time.Time
}

// Aggregate root collects events:
type User struct {
    // ...
    events []DomainEvent
}

func (u *User) DomainEvents() []DomainEvent { return u.events }
func (u *User) ClearDomainEvents()          { u.events = nil }

// Application handler publishes events after successful DB write:
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*CreateResult, error) {
    // ... create user ...
    
    // Dispatch domain events
    for _, event := range newUser.DomainEvents() {
        h.eventBus.Publish(ctx, event)
    }
    // ...
}
```

**Use cases untuk domain events:**
- `UserCreated` → trigger welcome email
- `AccountLocked` → trigger notification email
- `PasswordChanged` → trigger security notification
- `UserLoggedIn` → trigger suspicious login detection

---

### TASK-020: Tambahkan Special Character ke Password Policy

**Priority:** 🟢 P3 Low  
**Estimasi:** 30 menit  
**Files:** `infrastructure/password/service.go`

```go
func (s *Service) ValidatePolicy(password string) error {
    if len(password) < s.cfg.MinLength {
        return fmt.Errorf("password must be at least %d characters", s.cfg.MinLength)
    }
    
    var hasUpper, hasLower, hasNumber, hasSpecial bool
    for _, c := range password {
        switch {
        case unicode.IsUpper(c): hasUpper = true
        case unicode.IsLower(c): hasLower = true
        case unicode.IsDigit(c): hasNumber = true
        case unicode.IsPunct(c) || unicode.IsSymbol(c): hasSpecial = true
        }
    }
    
    if s.cfg.RequireUppercase && !hasUpper {
        return errors.New("password must contain at least one uppercase letter")
    }
    if s.cfg.RequireLowercase && !hasLower {
        return errors.New("password must contain at least one lowercase letter")
    }
    if s.cfg.RequireNumber && !hasNumber {
        return errors.New("password must contain at least one number")
    }
    // FIX: tambahkan ini
    if s.cfg.RequireSpecialChar && !hasSpecial {
        return errors.New("password must contain at least one special character (!@#$%^&*)")
    }
    
    return nil
}
```

---

## 8. Phase 5 — Observability & Operations

---

### TASK-021: Business Metrics (Prometheus)

**Priority:** 🟡 P2 Medium  
**Estimasi:** 1 hari  
**Files:** `delivery/grpc/metrics.go`

**Add business-level metrics:**

```go
// File: delivery/grpc/metrics.go — tambahkan:

var (
    // Auth metrics
    loginSuccessTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iam_login_success_total",
            Help: "Total successful logins",
        },
        []string{"service_name"},
    )
    loginFailureTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iam_login_failure_total",
            Help: "Total failed logins",
        },
        []string{"reason"},  // "invalid_password", "locked", "2fa_required", etc.
    )
    activeSessionsGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "iam_active_sessions",
            Help: "Current number of active sessions",
        },
    )
    lockedAccountsGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "iam_locked_accounts",
            Help: "Current number of locked user accounts",
        },
    )
    twoFAAdoptionGauge = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "iam_2fa_enabled_users",
            Help: "Number of users with 2FA enabled",
        },
    )
    passwordResetTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "iam_password_reset_total",
            Help: "Total password resets",
        },
    )
    tokenValidationTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "iam_token_validation_total",
            Help: "Total token validations (for introspection endpoint)",
        },
        []string{"service_name", "result"},  // result: "valid", "expired", "revoked"
    )
)
```

---

### TASK-022: CI/CD Pipeline untuk IAM Service

**Priority:** 🟡 P2 Medium  
**Estimasi:** 4 jam  
**Files:** `.github/workflows/iam-service.yml` (NEW)

```yaml
# File: .github/workflows/iam-service.yml

name: IAM Service CI/CD

on:
  push:
    branches: [main, develop]
    paths:
      - 'services/iam/**'
      - 'gen/**'
      - '.github/workflows/iam-service.yml'
  pull_request:
    branches: [main]
    paths:
      - 'services/iam/**'

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache-dependency-path: services/iam/go.sum
      - name: Run golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          working-directory: services/iam
          version: v2.3.0

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_DB: iam_test
          POSTGRES_USER: iam_user
          POSTGRES_PASSWORD: iam_pass
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
        ports:
          - 5432:5432
      redis:
        image: redis:7-alpine
        options: --health-cmd "redis-cli ping"
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
          cache-dependency-path: services/iam/go.sum
      - name: Run migrations
        working-directory: services/iam
        run: make migrate-up
        env:
          DATABASE_URL: postgres://iam_user:iam_pass@localhost:5432/iam_test
      - name: Run tests with coverage
        working-directory: services/iam
        run: |
          go test ./... -coverprofile=coverage.out -covermode=atomic
          go tool cover -func=coverage.out
        env:
          TEST_DATABASE_URL: postgres://iam_user:iam_pass@localhost:5432/iam_test
          TEST_REDIS_URL: localhost:6379
      - name: Coverage threshold check
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
          if (( $(echo "$COVERAGE < 70" | bc -l) )); then
            echo "Coverage $COVERAGE% is below 70% threshold"
            exit 1
          fi

  build:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        working-directory: services/iam
        run: |
          docker build -t iam-service:${{ github.sha }} .
          docker tag iam-service:${{ github.sha }} iam-service:latest
```

---

### TASK-023: Kubernetes Manifests untuk IAM

**Priority:** 🟢 P3 Low  
**Estimasi:** 2 jam  
**Files:** `services/iam/deployments/kubernetes/` (NEW)

Buat manifests lengkap seperti Finance service:
- `deployment.yaml`
- `service.yaml`
- `configmap.yaml`
- `secret.yaml.template`
- `hpa.yaml`
- `pdb.yaml`
- `networkpolicy.yaml`
- `rbac.yaml`

**Tambahan khusus IAM (karena menyimpan secret kriptografis):**
```yaml
# secret.yaml.template — tambahkan khusus IAM:
apiVersion: v1
kind: Secret
metadata:
  name: iam-secrets
type: Opaque
data:
  JWT_PRIVATE_KEY: <base64-encoded-RSA-private-key>
  TOTP_ENCRYPTION_KEY: <base64-encoded-32-byte-key>
  DB_PASSWORD: <base64-encoded-password>
  REDIS_PASSWORD: <base64-encoded-password>
```

---

## 9. Phase 6 — Testing & Documentation

---

### TASK-024: Unit Tests — Application Layer

**Priority:** 🟡 P2 Medium  
**Estimasi:** 3 hari

**Test files yang perlu dibuat:**

```
services/iam/internal/application/auth/
  ├── login_handler_test.go        ← WAJIB
  ├── refresh_handler_test.go      ← WAJIB  
  ├── forgot_password_handler_test.go
  ├── enable_2fa_handler_test.go
  └── validate_token_handler_test.go

services/iam/internal/application/user/
  ├── create_handler_test.go       ← WAJIB (terutama verify password hashing)
  ├── update_handler_test.go
  └── delete_handler_test.go

services/iam/internal/infrastructure/jwt/
  └── service_test.go              ← WAJIB

services/iam/internal/infrastructure/totp/
  └── service_test.go              ← WAJIB (verifikasi TOTP custom impl)
```

**Contoh test kritis:**

```go
// application/user/create_handler_test.go

func TestCreateHandler_PasswordIsHashed(t *testing.T) {
    // Setup mock dependencies
    mockUserRepo := &mocks.UserRepository{}
    mockPasswordSvc := &mocks.PasswordService{}
    
    // Setup expectations
    mockPasswordSvc.On("ValidatePolicy", "MyPassword123").Return(nil)
    mockPasswordSvc.On("Hash", "MyPassword123").Return("$argon2id$v=19$...", nil)
    mockUserRepo.On("ExistsUsername", mock.Anything, "testuser").Return(false, nil)
    mockUserRepo.On("ExistsEmail", mock.Anything, "test@example.com").Return(false, nil)
    mockUserRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *user.User) bool {
        // CRITICAL: Pastikan password yang disimpan adalah hash, bukan plaintext
        return strings.HasPrefix(u.PasswordHash(), "$argon2id$")
    })).Return(nil)
    
    handler := NewCreateHandler(mockUserRepo, mockPasswordSvc, ...)
    
    _, err := handler.Handle(ctx, CreateCommand{
        Username: "testuser",
        Email:    "test@example.com",
        Password: "MyPassword123",  // plain text input
    })
    
    require.NoError(t, err)
    mockPasswordSvc.AssertCalled(t, "Hash", "MyPassword123")
    mockUserRepo.AssertCalled(t, "Create", mock.Anything, mock.MatchedBy(func(u *user.User) bool {
        return u.PasswordHash() != "MyPassword123"  // TIDAK boleh plaintext
    }))
}
```

---

### TASK-025: Integration Tests — Session & Auth Flow

**Priority:** 🟡 P2 Medium  
**Estimasi:** 2 hari

```go
// tests/e2e/session_test.go (NEW)

func TestMultiDeviceSession(t *testing.T) { ... }
func TestSessionCleanup(t *testing.T)     { ... }
func TestRefreshTokenRotation(t *testing.T) { ... }
func TestTokenRevocationCrossService(t *testing.T) { ... }
```

---

### TASK-026: Update Documentation

**Priority:** 🟢 P3 Low  
**Estimasi:** 1 hari

1. Update root `README.md` untuk include IAM service
2. Buat `services/iam/docs/API.md` dengan semua endpoint, request/response examples
3. Buat `services/iam/docs/SSO_INTEGRATION.md` — panduan untuk services lain integrasi dengan IAM
4. Buat `services/iam/docs/SECURITY.md` — security model documentation
5. Tambahkan `.gitignore` untuk IAM service
6. Dokumentasikan seeds/main.go

---

## 10. Detail Teknis Per Item

### Dependency Injection — Wiring di main.go

Untuk semua handler baru, perlu update `cmd/server/main.go`:

```go
// cmd/server/main.go — Updated dependency wiring

func main() {
    // Infrastructure
    db := postgres.NewConnection(cfg.Database)
    redisClient := redis.NewClient(cfg.Redis)
    
    // JWT Service (RS256)
    jwtSvc := jwt.NewService(cfg.JWT)  // menggunakan private key
    
    // Encryption Service (untuk TOTP)
    encryptSvc := crypto.NewEncryptionService(cfg.Encryption.TOTPSecretKey)
    
    // Repositories
    userRepo := postgres.NewUserRepository(db)
    sessionRepo := postgres.NewSessionRepository(db)
    sessionCacheRepo := redis.NewSessionCacheRepository(redisClient)
    apiKeyRepo := postgres.NewAPIKeyRepository(db)  // NEW
    passwordResetRepo := postgres.NewPasswordResetRepository(db)  // NEW
    // ...
    
    // Application Handlers — Auth
    loginHandler := auth.NewLoginHandler(userRepo, sessionRepo, sessionCacheRepo, 
                                         jwtSvc, passwordSvc, totpSvc, auditRepo, cfg)
    logoutHandler := auth.NewLogoutHandler(sessionRepo, sessionCacheRepo, jwtSvc, auditRepo)
    refreshHandler := auth.NewRefreshTokenHandler(sessionRepo, sessionCacheRepo, jwtSvc)
    forgotPwdHandler := auth.NewForgotPasswordHandler(userRepo, passwordResetRepo, 
                                                       sessionCacheRepo, emailSvc, cfg)
    validateTokenHandler := auth.NewValidateTokenHandler(jwtSvc, sessionCacheRepo)  // NEW
    // ...
    
    // Schedulers
    sessionCleanup := scheduler.NewSessionCleanupJob(sessionRepo, 24*time.Hour)
    go sessionCleanup.Start(ctx)
    
    // gRPC Server
    authHandler := grpc.NewAuthHandler(loginHandler, logoutHandler, refreshHandler, 
                                        forgotPwdHandler, validateTokenHandler, ...)
    // ...
}
```

---

## 11. Migration Strategy

### Database Migrations yang Diperlukan

```sql
-- 000008: Multi-device session support
-- File: migrations/postgres/000008_update_session_multi_device.up.sql

ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS device_type VARCHAR(50) DEFAULT 'web';

DROP INDEX IF EXISTS idx_user_active_session;

CREATE UNIQUE INDEX idx_user_service_active_session
    ON user_sessions(user_id, service_name)
    WHERE revoked_at IS NULL;

-- 000009: API Key enhancements (schema sudah ada, tidak perlu create)
-- File: migrations/postgres/000009_api_keys_indexes.up.sql

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_service ON api_keys(service_name) WHERE is_active = TRUE;

-- 000010: Password reset token improvements  
-- File: migrations/postgres/000010_password_reset_enhancement.up.sql

-- Hash the OTP code (currently stored plaintext in column otp_code)
ALTER TABLE password_reset_tokens ADD COLUMN IF NOT EXISTS otp_hash VARCHAR(64);
UPDATE password_reset_tokens SET otp_hash = encode(sha256(otp_code::bytea), 'hex') 
    WHERE otp_hash IS NULL;
-- Setelah verifikasi: DROP COLUMN otp_code

-- 000011: Session device_info dan cleanup
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW();
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON user_sessions(expires_at) 
    WHERE revoked_at IS NULL;

-- 000012: Audit log enhancements
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS session_id UUID REFERENCES user_sessions(session_id);
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS api_key_id UUID REFERENCES api_keys(key_id);
```

### Data Migrations (Non-Schema)

```go
// scripts/data_migration/001_encrypt_totp_secrets.go
// Jalankan setelah deploy TASK-004

// scripts/data_migration/002_upgrade_bcrypt_hashes.go  
// Jalankan untuk batch upgrade (tidak diperlukan jika TASK-012 sudah ada)

// scripts/data_migration/003_seed_permissions.go
// Seed semua permission codes untuk IAM, Finance, HR services
```

---

## 12. Dependency Additions

### IAM Service (go.mod)

```go
// Tambahkan:
require (
    // Excel export/import (sudah ada di Finance, belum di IAM)
    github.com/xuri/excelize/v2 v2.8.1
    
    // TOTP library (menggantikan custom implementation)
    github.com/pquerna/otp v1.4.0
    
    // RSA key generation/parsing helpers
    // (sudah tersedia di stdlib crypto/rsa, tidak perlu library baru)
    
    // Rate limiting (sudah ada golang.org/x/time/rate)
    
    // Scheduler (cukup dengan stdlib time.Ticker)
    
    // Email dengan HTML templates (sudah ada net/smtp di stdlib)
    // Untuk retry/queue:
    github.com/hibiken/asynq v0.24.1  // opsional, untuk async email queue
)

// Hapus (jika ada):
// Tidak ada dependency yang perlu dihapus dari IAM
```

---

## 13. Estimasi Total

### Per Phase

| Phase | Task | Estimasi | Sprint |
|-------|------|----------|--------|
| **Phase 0** | TASK-001 sampai 003 | 1 hari | Sprint 1 |
| **Phase 1** | TASK-004 sampai 007 | 8 hari | Sprint 2-3 |
| **Phase 2** | TASK-008 sampai 012 | 6 hari | Sprint 4-5 |
| **Phase 3** | TASK-013 sampai 016 | 12 hari | Sprint 6-8 |
| **Phase 4** | TASK-017 sampai 020 | 8 hari | Sprint 9-10 |
| **Phase 5** | TASK-021 sampai 023 | 7 hari | Sprint 11 |
| **Phase 6** | TASK-024 sampai 026 | 6 hari | Sprint 12 |
| **Total** | 26 tasks | ~48 hari | 12 sprint |

### Per Prioritas

| Prioritas | Task Count | Hari | Keterangan |
|-----------|-----------|------|-----------|
| 🔴 P0 Critical | 3 | ~1 hari | Fix SEKARANG |
| 🔴 P1 High | 4 | ~10 hari | Sebelum production |
| 🟡 P2 Medium | 12 | ~25 hari | Feature complete |
| 🟢 P3 Low | 7 | ~12 hari | Nice to have |

### Milestone Kunci

```
Week 1: Phase 0 selesai → Service aman dari bug kritis
Week 3: Phase 1 selesai → JWT RS256 + Token Introspection deployed
Week 5: Phase 2 selesai → SSO architecture lengkap
Week 8: Phase 3 selesai → Semua fitur bulk export/import
Week 10: Phase 4 selesai → Code quality & consistency
Week 12: Phase 5+6 selesai → Fully observable, tested, documented
```

---

## Appendix: Checklist Quick Reference

### Security Checklist (Pre-Production)

- [ ] TASK-001: Verified password hashing in CreateUser
- [ ] TASK-002: Rate limiter per-method fixed
- [ ] TASK-003: Session token hash consistency fixed
- [ ] TASK-004: TOTP secrets encrypted at rest
- [ ] TASK-005: JWT migrated to RS256
- [ ] TASK-006: Token introspection endpoint live
- [ ] TASK-007: API keys implemented
- [ ] All gRPC communication behind TLS (production)
- [ ] All secrets in Kubernetes secrets (not configmap)
- [ ] Password expiry policy enabled

### SSO Readiness Checklist

- [ ] RS256 JWT + JWKS endpoint (`/.well-known/jwks.json`)
- [ ] OpenID Connect Discovery (`/.well-known/openid-configuration`)
- [ ] Token Introspection gRPC endpoint
- [ ] API Keys for service-to-service
- [ ] Multi-service permission format (`service.module.entity.action`)
- [ ] Multi-device session support
- [ ] Cross-service audit trail (service_name di audit logs)
- [ ] Centralized logout (invalidate tokens across all services)

### Feature Completeness Checklist

- [ ] All 29 stub endpoints implemented
- [ ] Password expiry and warning
- [ ] Account lockout notification email
- [ ] Welcome email for new users
- [ ] Suspicious login detection
- [ ] HTML email templates
- [ ] Bcrypt → Argon2id auto-migration
- [ ] Session cleanup scheduler

### Code Quality Checklist

- [ ] Auth service refactored to individual handlers
- [ ] Menu/Organization refactored to handler pattern
- [ ] Unit test coverage ≥ 70%
- [ ] Application layer 100% unit tested
- [ ] All integration tests passing
- [ ] CI/CD pipeline live for IAM service
- [ ] golangci-lint passing with 0 errors

---

*Dokumen ini adalah living document — update seiring implementasi berlangsung.*  
*Referensi temuan detail ada di `IAM_SERVICE_FINDINGS.md`.*
