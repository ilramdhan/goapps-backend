// Package grpc provides gRPC server implementation.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
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

// AuthInterceptor validates JWT tokens issued by IAM service.
// blacklist is optional — if nil, blacklist checking is skipped (graceful degradation).
func AuthInterceptor(cfg *config.JWTConfig, blacklist TokenBlacklistChecker) grpc.UnaryServerInterceptor {
	secret := []byte(cfg.AccessTokenSecret)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Health checks are always public.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		// All finance endpoints require authentication.
		token, err := extractBearerToken(ctx)
		if err != nil {
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
				// Short access token TTL (15min) limits exposure.
			}
			if blacklisted {
				return nil, status.Error(codes.Unauthenticated, "token has been revoked")
			}
		}

		// Populate context with user info from claims.
		ctx = context.WithValue(ctx, AuthUserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, AuthUsernameKey, claims.Username)
		ctx = context.WithValue(ctx, AuthEmailKey, claims.Email)
		ctx = context.WithValue(ctx, AuthRolesKey, claims.Roles)
		ctx = context.WithValue(ctx, AuthPermissionsKey, claims.Permissions)

		return handler(ctx, req)
	}
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

// PermissionInterceptor enforces RBAC permission checks for Finance service methods.
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
			// No specific permission needed — authenticated access is sufficient.
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

// getRequiredPermission returns the permission code needed for a method.
func getRequiredPermission(fullMethod string) string {
	// Permission mapping for Finance service.
	// Format: {service}.{module}.{entity}.{action}
	permissions := map[string]string{
		// UOM Service
		"/finance.v1.UOMService/CreateUOM": "finance.master.uom.create",
		"/finance.v1.UOMService/GetUOM":    "finance.master.uom.view",
		"/finance.v1.UOMService/ListUOMs":  "finance.master.uom.view",
		"/finance.v1.UOMService/UpdateUOM": "finance.master.uom.update",
		"/finance.v1.UOMService/DeleteUOM": "finance.master.uom.delete",
		"/finance.v1.UOMService/ImportUOM": "finance.master.uom.create",
		"/finance.v1.UOMService/ExportUOM": "finance.master.uom.view",

		// RM Category Service
		"/finance.v1.RMCategoryService/CreateRMCategory":           "finance.master.rmcategory.create",
		"/finance.v1.RMCategoryService/GetRMCategory":              "finance.master.rmcategory.view",
		"/finance.v1.RMCategoryService/ListRMCategories":           "finance.master.rmcategory.view",
		"/finance.v1.RMCategoryService/UpdateRMCategory":           "finance.master.rmcategory.update",
		"/finance.v1.RMCategoryService/DeleteRMCategory":           "finance.master.rmcategory.delete",
		"/finance.v1.RMCategoryService/ImportRMCategories":         "finance.master.rmcategory.import",
		"/finance.v1.RMCategoryService/ExportRMCategories":         "finance.master.rmcategory.export",
		"/finance.v1.RMCategoryService/DownloadRMCategoryTemplate": "finance.master.rmcategory.view",
	}

	return permissions[fullMethod]
}
