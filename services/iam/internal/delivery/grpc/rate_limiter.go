// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"context"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	refillRate   float64 // tokens per second
	lastRefill   time.Time
	methodLimits map[string]float64
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(requestsPerSecond float64) *RateLimiter {
	return &RateLimiter{
		tokens:     requestsPerSecond,
		maxTokens:  requestsPerSecond * 2, // Allow burst
		refillRate: requestsPerSecond,
		lastRefill: time.Now(),
		methodLimits: map[string]float64{
			// Auth: strict limits to prevent brute force
			"/iam.v1.AuthService/Login":          5,
			"/iam.v1.AuthService/ForgotPassword": 5,
			"/iam.v1.AuthService/VerifyResetOTP": 5,
			"/iam.v1.AuthService/ResetPassword":  5,
			"/iam.v1.AuthService/RefreshToken":   20,
			"/iam.v1.AuthService/Logout":         20,
			"/iam.v1.AuthService/GetCurrentUser": 50,
			"/iam.v1.AuthService/UpdatePassword": 5,
			"/iam.v1.AuthService/Enable2FA":      5,
			"/iam.v1.AuthService/Verify2FA":      5,
			"/iam.v1.AuthService/Disable2FA":     5,
			// User: moderate limits
			"/iam.v1.UserService/ListUsers":     50,
			"/iam.v1.UserService/GetUser":       100,
			"/iam.v1.UserService/GetUserDetail": 100,
			"/iam.v1.UserService/CreateUser":    10,
			"/iam.v1.UserService/UpdateUser":    10,
			"/iam.v1.UserService/DeleteUser":    10,
			"/iam.v1.UserService/ImportUsers":   2,
			"/iam.v1.UserService/ExportUsers":   5,
			// Role/Permission: moderate limits
			"/iam.v1.RoleService/ListRoles":             50,
			"/iam.v1.RoleService/GetRole":               100,
			"/iam.v1.RoleService/CreateRole":            10,
			"/iam.v1.RoleService/UpdateRole":            10,
			"/iam.v1.RoleService/DeleteRole":            10,
			"/iam.v1.PermissionService/ListPermissions": 50,
			// Menu: moderate limits
			"/iam.v1.MenuService/GetMenuTree":     50,
			"/iam.v1.MenuService/GetFullMenuTree": 50,
			// Session: moderate limits
			"/iam.v1.SessionService/ListActiveSessions": 50,
			// Audit: moderate limits
			"/iam.v1.AuditService/ListAuditLogs": 50,
		},
	}
}

// Allow checks if a request is allowed.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	// Check if we have tokens
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// RateLimitInterceptor creates a rate limiting interceptor.
func RateLimitInterceptor(limiter *RateLimiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded, please try again later")
		}
		return handler(ctx, req)
	}
}
