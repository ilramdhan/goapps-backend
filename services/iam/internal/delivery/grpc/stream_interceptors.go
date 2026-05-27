package grpc

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/jwt"
	redisinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/redis"
)

// wrappedStream wraps grpc.ServerStream so we can override the context.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the (possibly augmented) context.
func (w *wrappedStream) Context() context.Context { return w.ctx }

// AuthStreamInterceptor mirrors AuthInterceptor for server-streaming RPCs.
// It validates the JWT bearer token, populates the user context, and rejects
// requests that target a non-public method without a valid token.
// internalToken accepts the shared `x-internal-token` bypass.
func AuthStreamInterceptor(jwtService *jwt.Service, sessionCache *redisinfra.SessionCache, sessionRepo session.Repository, internalToken string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if isPublicMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		if internalToken != "" && hasInternalToken(ctx, internalToken) {
			ctx = context.WithValue(ctx, UserIDKey, "system")
			ctx = context.WithValue(ctx, UsernameKey, "system")
			ctx = context.WithValue(ctx, RolesKey, []string{"system"})
			ctx = context.WithValue(ctx, PermissionsKey, []string{})
			return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
		}

		token, err := extractBearerToken(ctx)
		if err != nil {
			log.Debug().Str("method", info.FullMethod).Err(err).Msg("Stream auth failed: no token")
			return status.Error(codes.Unauthenticated, "authentication required")
		}
		claims, err := jwtService.ValidateAccessToken(token)
		if err != nil {
			log.Debug().Str("method", info.FullMethod).Err(err).Msg("Stream auth failed: invalid token")
			return status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		if checkTokenBlacklist(ctx, sessionCache, claims.ID) {
			return status.Error(codes.Unauthenticated, "token has been revoked")
		}

		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UsernameKey, claims.Username)
		ctx = context.WithValue(ctx, EmailKey, claims.Email)
		ctx = context.WithValue(ctx, RolesKey, claims.Roles)
		ctx = context.WithValue(ctx, PermissionsKey, claims.Permissions)

		updateSessionActivity(sessionRepo, claims.UserID)

		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// PermissionStreamInterceptor mirrors PermissionInterceptor for streaming RPCs.
func PermissionStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if isPublicMethod(info.FullMethod) {
			return handler(srv, ss)
		}
		if IsSuperAdmin(ctx) {
			return handler(srv, ss)
		}
		req, exists := methodPermissions[info.FullMethod]
		if !exists {
			log.Warn().Str("method", info.FullMethod).Msg("Stream permission: unmapped method denied")
			return status.Error(codes.PermissionDenied, "access denied")
		}
		if req.Permission == "" {
			return handler(srv, ss)
		}
		if !HasPermission(ctx, req.Permission) {
			return status.Error(codes.PermissionDenied, "insufficient permissions")
		}
		return handler(srv, ss)
	}
}

// StreamRecoveryInterceptor recovers from panics in stream handlers.
func StreamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Any("panic", r).Str("method", info.FullMethod).Msg("Stream handler panic recovered")
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
