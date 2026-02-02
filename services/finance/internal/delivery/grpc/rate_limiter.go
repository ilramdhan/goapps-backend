// Package grpc provides gRPC server implementation for finance service.
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
			// Higher limits for read operations
			"/finance.v1.UOMService/ListUOMs": 50,
			"/finance.v1.UOMService/GetUOM":   100,
			// Lower limits for write operations
			"/finance.v1.UOMService/CreateUOM": 10,
			"/finance.v1.UOMService/UpdateUOM": 10,
			"/finance.v1.UOMService/DeleteUOM": 10,
			// Very low for expensive operations
			"/finance.v1.UOMService/ImportUOMs": 2,
			"/finance.v1.UOMService/ExportUOMs": 5,
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
