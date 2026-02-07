// Package redis provides Redis cache implementations.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

// Client wraps the Redis client.
type Client struct {
	*redis.Client
}

// NewClient creates a new Redis client.
func NewClient(cfg *config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address(),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	log.Info().
		Str("address", cfg.Address()).
		Int("db", cfg.DB).
		Msg("Redis connection established")

	return &Client{client}, nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	if err := c.Client.Close(); err != nil {
		return fmt.Errorf("failed to close redis: %w", err)
	}
	log.Info().Msg("Redis connection closed")
	return nil
}

// SessionCache implements session.CacheRepository interface.
type SessionCache struct {
	client    *Client
	ttl       time.Duration
	blacklist time.Duration
}

// NewSessionCache creates a new SessionCache.
func NewSessionCache(client *Client, cfg *config.RedisConfig) *SessionCache {
	return &SessionCache{
		client:    client,
		ttl:       cfg.SessionTTL,
		blacklist: cfg.TokenBlacklistTTL,
	}
}

const (
	sessionPrefix   = "iam:session:"
	blacklistPrefix = "iam:blacklist:"
)

// StoreSession stores session data in cache for quick validation.
func (c *SessionCache) StoreSession(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID, expiresIn int64) error {
	key := sessionPrefix + sessionID.String()
	return c.client.Set(ctx, key, userID.String(), time.Duration(expiresIn)*time.Second).Err()
}

// GetSession retrieves session data from cache.
func (c *SessionCache) GetSession(ctx context.Context, sessionID uuid.UUID) (uuid.UUID, error) {
	key := sessionPrefix + sessionID.String()
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}
	return uuid.Parse(val)
}

// DeleteSession removes a session from cache.
func (c *SessionCache) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	key := sessionPrefix + sessionID.String()
	return c.client.Del(ctx, key).Err()
}

// BlacklistToken adds a token to the blacklist (for logout before expiry).
func (c *SessionCache) BlacklistToken(ctx context.Context, tokenID string, expiresIn int64) error {
	key := blacklistPrefix + tokenID
	return c.client.Set(ctx, key, "1", time.Duration(expiresIn)*time.Second).Err()
}

// IsBlacklisted checks if a token is blacklisted.
func (c *SessionCache) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := blacklistPrefix + tokenID
	exists, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}

// OTPCache provides OTP storage in Redis.
type OTPCache struct {
	client *Client
	ttl    time.Duration
}

// NewOTPCache creates a new OTPCache.
func NewOTPCache(client *Client, ttl time.Duration) *OTPCache {
	return &OTPCache{
		client: client,
		ttl:    ttl,
	}
}

const (
	otpPrefix          = "iam:otp:"
	resetTokenPrefix   = "iam:reset:"
	loginAttemptPrefix = "iam:login_attempt:"
)

// StoreOTP stores an OTP code for a user.
func (c *OTPCache) StoreOTP(ctx context.Context, userID uuid.UUID, otp string) error {
	key := otpPrefix + userID.String()
	return c.client.Set(ctx, key, otp, c.ttl).Err()
}

// VerifyOTP verifies and deletes an OTP code.
func (c *OTPCache) VerifyOTP(ctx context.Context, userID uuid.UUID, otp string) (bool, error) {
	key := otpPrefix + userID.String()
	stored, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}

	if stored != otp {
		return false, nil
	}

	// Delete after successful verification
	_ = c.client.Del(ctx, key)
	return true, nil
}

// StoreResetToken stores a password reset token.
func (c *OTPCache) StoreResetToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error {
	key := resetTokenPrefix + token
	return c.client.Set(ctx, key, userID.String(), ttl).Err()
}

// GetResetToken retrieves and deletes a reset token.
func (c *OTPCache) GetResetToken(ctx context.Context, token string) (uuid.UUID, error) {
	key := resetTokenPrefix + token
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, nil
		}
		return uuid.Nil, err
	}

	// Delete after retrieval
	_ = c.client.Del(ctx, key)
	return uuid.Parse(val)
}

// RateLimitCache provides rate limiting functionality.
type RateLimitCache struct {
	client *Client
}

// NewRateLimitCache creates a new RateLimitCache.
func NewRateLimitCache(client *Client) *RateLimitCache {
	return &RateLimitCache{client: client}
}

// IncrementLoginAttempt increments the login attempt counter.
func (c *RateLimitCache) IncrementLoginAttempt(ctx context.Context, identifier string, ttl time.Duration) (int64, error) {
	key := loginAttemptPrefix + identifier
	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// GetLoginAttempts gets the current login attempt count.
func (c *RateLimitCache) GetLoginAttempts(ctx context.Context, identifier string) (int64, error) {
	key := loginAttemptPrefix + identifier
	val, err := c.client.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return val, nil
}

// ResetLoginAttempts resets the login attempt counter.
func (c *RateLimitCache) ResetLoginAttempts(ctx context.Context, identifier string) error {
	key := loginAttemptPrefix + identifier
	return c.client.Del(ctx, key).Err()
}
