// Package redis provides Redis connection and caching utilities.
package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

const (
	blacklistPrefix   = "iam:blacklist:"
	permissionsPrefix = "iam:user_perms:"
)

// TokenBlacklist checks the shared IAM token blacklist in Redis.
// This enables cross-service logout enforcement.
type TokenBlacklist struct {
	client *redis.Client
}

// NewTokenBlacklist creates a new token blacklist checker connected to the IAM Redis.
func NewTokenBlacklist(cfg *config.AuthRedisConfig) (*TokenBlacklist, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Address(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     5,
		MinIdleConns: 1,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to auth redis: %w", err)
	}

	log.Info().
		Str("address", cfg.Address()).
		Int("db", cfg.DB).
		Msg("Auth Redis (token blacklist) connection established")

	return &TokenBlacklist{client: client}, nil
}

// IsBlacklisted checks if a token JTI is on the blacklist.
func (tb *TokenBlacklist) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := blacklistPrefix + tokenID
	exists, err := tb.client.Exists(ctx, key).Result()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, fmt.Errorf("blacklist check failed: %w", err)
	}
	return exists > 0, nil
}

// GetUserPermissions retrieves cached permissions for a user from the IAM Redis.
// Returns nil, nil on cache miss (fail-open).
func (tb *TokenBlacklist) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	key := permissionsPrefix + userID
	val, err := tb.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}
	if val == "" {
		return nil, nil
	}
	return strings.Split(val, "|"), nil
}

// Close closes the auth Redis connection.
func (tb *TokenBlacklist) Close() error {
	return tb.client.Close()
}
