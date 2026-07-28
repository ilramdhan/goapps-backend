// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/config"
)

// Auth context keys.
const (
	AuthUserIDKey      ContextKey = "auth_user_id"
	AuthUsernameKey    ContextKey = "auth_username"
	AuthEmailKey       ContextKey = "auth_email"
	AuthRolesKey       ContextKey = "auth_roles"
	AuthPermissionsKey ContextKey = "auth_permissions"
)

// JWTClaims mirrors the IAM service JWT claims structure.
type JWTClaims struct {
	jwt.RegisteredClaims
	TokenType   string   `json:"token_type"`
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// TokenBlacklistChecker checks if a token has been revoked.
type TokenBlacklistChecker interface {
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}

// PermissionsReader fetches a user's permissions from the IAM Redis cache.
type PermissionsReader interface {
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

// serviceSecretValid reports whether the internal x-service-secret header
// matches the configured secret. An empty configured secret skips the check.
func serviceSecretValid(ctx context.Context, svcSecret string) bool {
	if svcSecret == "" {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	vals := md.Get("x-service-secret")
	return len(vals) == 1 && vals[0] == svcSecret
}

// AuthInterceptor validates JWT tokens issued by the IAM service.
// blacklist is optional — if nil, blacklist checking is skipped.
// permsReader is optional — if nil, permissions fall back to JWT claims.
func AuthInterceptor(cfg *config.JWTConfig, blacklist TokenBlacklistChecker, permsReader PermissionsReader) grpc.UnaryServerInterceptor { //nolint:gocognit,gocyclo // sequential auth gates, cohesive
	secret := []byte(cfg.AccessTokenSecret)
	svcSecret := cfg.ServiceSecret

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Health checks and reflection are always public.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		// All PPC endpoints require authentication.
		token, err := extractBearerToken(ctx)
		if err != nil {
			// Fall back to service-secret bypass for trusted internal callers.
			if serviceSecretValid(ctx, svcSecret) && svcSecret != "" {
				return handler(withServiceIdentity(ctx), req)
			}
			return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization: %v", err)
		}

		claims, err := validateAccessToken(token, secret)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Check token blacklist (cross-service logout enforcement).
		if blacklist != nil && claims.ID != "" {
			blacklisted, blErr := blacklist.IsBlacklisted(ctx, claims.ID)
			if blErr != nil {
				log.Warn().Err(blErr).Msg("Failed to check token blacklist")
				// Fail-open: continue if blacklist check fails.
			}
			if blacklisted {
				return nil, status.Error(codes.Unauthenticated, "token has been revoked")
			}
		}

		// Resolve permissions from IAM Redis cache; fall back to JWT claims.
		perms := claims.Permissions
		if permsReader != nil {
			if cached, err := permsReader.GetUserPermissions(ctx, claims.UserID); err != nil {
				log.Warn().Err(err).Str("userID", claims.UserID).Msg("failed to fetch permissions from Redis, using JWT fallback")
			} else if cached != nil {
				perms = cached
			}
		}

		ctx = context.WithValue(ctx, AuthUserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, AuthUsernameKey, claims.Username)
		ctx = context.WithValue(ctx, AuthEmailKey, claims.Email)
		ctx = context.WithValue(ctx, AuthRolesKey, claims.Roles)
		ctx = context.WithValue(ctx, AuthPermissionsKey, perms)

		return handler(ctx, req)
	}
}

// withServiceIdentity injects a synthetic SUPER_ADMIN identity for trusted
// service-to-service calls so the permission interceptor's bypass applies.
func withServiceIdentity(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, AuthUserIDKey, "service:ppc-internal")
	ctx = context.WithValue(ctx, AuthUsernameKey, "ppc-internal")
	ctx = context.WithValue(ctx, AuthRolesKey, []string{"SUPER_ADMIN"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{})
	return ctx
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("no authorization header")
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("invalid authorization format")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

func validateAccessToken(tokenString string, secret []byte) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token expired")
		}
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if claims.TokenType != "access" {
		return nil, errors.New("not an access token")
	}

	return claims, nil
}

// GetUserIDFromCtx retrieves the user ID from context.
func GetUserIDFromCtx(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AuthUserIDKey).(string)
	return val, ok
}

// GetUsernameFromCtx retrieves the username from context.
func GetUsernameFromCtx(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AuthUsernameKey).(string)
	return val, ok
}

// GetRolesFromCtx retrieves the roles from context.
func GetRolesFromCtx(ctx context.Context) []string {
	roles, ok := ctx.Value(AuthRolesKey).([]string)
	if !ok {
		return nil
	}
	return roles
}

// GetPermissionsFromCtx retrieves the permissions from context.
func GetPermissionsFromCtx(ctx context.Context) []string {
	perms, ok := ctx.Value(AuthPermissionsKey).([]string)
	if !ok {
		return nil
	}
	return perms
}

// HasPermission checks if the user has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
	return slices.Contains(GetPermissionsFromCtx(ctx), permission)
}

// HasRole checks if the user has a specific role.
func HasRole(ctx context.Context, role string) bool {
	return slices.Contains(GetRolesFromCtx(ctx), role)
}

// IsSuperAdmin checks if the user has the SUPER_ADMIN role.
func IsSuperAdmin(ctx context.Context) bool {
	return HasRole(ctx, "SUPER_ADMIN")
}

// PermissionInterceptor enforces RBAC permission checks for PPC service methods.
func PermissionInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Skip for health/reflection.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		// SUPER_ADMIN bypasses all permission checks.
		if IsSuperAdmin(ctx) {
			return handler(ctx, req)
		}

		required := getRequiredPermission(info.FullMethod)
		if required == "" {
			// No specific permission mapped yet — authenticated access is sufficient.
			return handler(ctx, req)
		}

		if !HasPermission(ctx, required) {
			log.Warn().
				Str("method", info.FullMethod).
				Str("required", required).
				Msg("Permission denied")
			return nil, status.Errorf(codes.PermissionDenied, "permission denied: requires %s", required)
		}

		return handler(ctx, req)
	}
}

// rpcPermissions maps a fully-qualified gRPC method to the ppc.* permission
// code it requires. Intentionally empty during Phase-1 scaffolding; per-RPC
// mappings are added alongside the business handlers (plan-03+).
var rpcPermissions = map[string]string{}

// getRequiredPermission returns the permission code needed for a method, or ""
// when the method requires only authenticated access.
func getRequiredPermission(fullMethod string) string {
	return rpcPermissions[fullMethod]
}
