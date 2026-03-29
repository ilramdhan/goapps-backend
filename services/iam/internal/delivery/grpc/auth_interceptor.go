// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
)

// Additional context keys for auth data.
const (
	UsernameKey    ContextKey = "username"
	EmailKey       ContextKey = "email"
	RolesKey       ContextKey = "roles"
	PermissionsKey ContextKey = "permissions"
)

// publicMethods lists gRPC methods that do not require authentication.
var publicMethods = map[string]bool{
	"/iam.v1.AuthService/Login":          true,
	"/iam.v1.AuthService/RefreshToken":   true,
	"/iam.v1.AuthService/ForgotPassword": true,
	"/iam.v1.AuthService/VerifyResetOTP": true,
	"/iam.v1.AuthService/ResetPassword":  true,
	"/iam.v1.AuthService/Logout":         true,
	"/grpc.health.v1.Health/Check":       true,
	"/grpc.health.v1.Health/Watch":       true,

	// CMS public endpoints (no auth required)
	"/iam.v1.CMSSectionService/GetPublicLandingContent": true,
	"/iam.v1.CMSPageService/GetCMSPageBySlug":           true,
}

// isPublicMethod checks if a gRPC method is public (no auth required).
func isPublicMethod(fullMethod string) bool {
	if publicMethods[fullMethod] {
		return true
	}
	// Allow reflection in non-production environments
	if strings.HasPrefix(fullMethod, "/grpc.reflection.") {
		return true
	}
	return false
}

// checkTokenBlacklist checks if the token is blacklisted in Redis.
// Returns true if the token is revoked. Fails open if Redis is unavailable.
func checkTokenBlacklist(ctx context.Context, cache *redis.SessionCache, tokenID string) bool {
	if cache == nil {
		return false
	}
	blacklisted, err := cache.IsBlacklisted(ctx, tokenID)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check token blacklist")
		return false
	}
	return blacklisted
}

// updateSessionActivity fires a non-blocking goroutine to update session last_activity_at.
func updateSessionActivity(repo session.Repository, userIDStr string) {
	if repo == nil {
		return
	}
	go func() {
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return
		}
		if updateErr := repo.UpdateActivity(context.Background(), userID); updateErr != nil {
			log.Debug().Err(updateErr).Msg("failed to update session activity")
		}
	}()
}

// AuthInterceptor creates a unary interceptor that validates JWT tokens
// and populates the context with user information.
// sessionRepo is optional — when provided, it updates last_activity_at on each request.
func AuthInterceptor(jwtService *jwt.Service, sessionCache *redis.SessionCache, sessionRepo session.Repository) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Skip auth for public endpoints
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract token from metadata
		token, err := extractBearerToken(ctx)
		if err != nil {
			log.Debug().
				Str("method", info.FullMethod).
				Err(err).
				Msg("Authentication failed: no token")
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}

		// Validate access token
		claims, err := jwtService.ValidateAccessToken(token)
		if err != nil {
			log.Debug().
				Str("method", info.FullMethod).
				Err(err).
				Msg("Authentication failed: invalid token")
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}

		// Check token blacklist
		if checkTokenBlacklist(ctx, sessionCache, claims.ID) {
			return nil, status.Error(codes.Unauthenticated, "token has been revoked")
		}

		// Populate context with user information from JWT claims
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, EmailKey, claims.Email)
		ctx = context.WithValue(ctx, RolesKey, claims.Roles)
		ctx = context.WithValue(ctx, PermissionsKey, claims.Permissions)

		// Update session last_activity_at (fire-and-forget, non-blocking)
		updateSessionActivity(sessionRepo, claims.UserID)

		return handler(ctx, req)
	}
}

// extractBearerToken extracts the Bearer token from gRPC metadata.
func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	// Check "authorization" header (standard gRPC metadata is lowercase)
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", status.Error(codes.Unauthenticated, "invalid authorization format")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "empty token")
	}

	return token, nil
}

// GetUserIDFromCtx extracts user ID string from context (set by AuthInterceptor).
func GetUserIDFromCtx(ctx context.Context) (string, bool) {
	if v, ok := ctx.Value(UserIDKey).(string); ok && v != "" {
		return v, true
	}
	return "", false
}

// GetUsernameFromCtx extracts username from context.
func GetUsernameFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(UsernameKey).(string); ok {
		return v
	}
	return ""
}

// GetRolesFromCtx extracts roles from context.
func GetRolesFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(RolesKey).([]string); ok {
		return v
	}
	return nil
}

// GetPermissionsFromCtx extracts permissions from context.
func GetPermissionsFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(PermissionsKey).([]string); ok {
		return v
	}
	return nil
}

// HasRole checks if the user in context has a specific role.
func HasRole(ctx context.Context, role string) bool {
	return slices.Contains(GetRolesFromCtx(ctx), role)
}

// HasPermission checks if the user in context has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
	return slices.Contains(GetPermissionsFromCtx(ctx), permission)
}

// IsSuperAdmin checks if the user has the SUPER_ADMIN role.
func IsSuperAdmin(ctx context.Context) bool {
	return HasRole(ctx, "SUPER_ADMIN")
}
