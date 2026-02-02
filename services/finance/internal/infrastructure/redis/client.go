// Package redis provides Redis connection and caching utilities.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Client wraps the Redis client with additional functionality.
type Client struct {
	client *redis.Client
	prefix string
}

// NewClient creates a new Redis client.
func NewClient(cfg *config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{
		client: client,
		prefix: "finance:",
	}, nil
}

// Close closes the Redis client.
func (c *Client) Close() error {
	return c.client.Close()
}

// Ping checks if Redis is available.
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Get retrieves a value from cache.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, c.prefix+key).Result()
}

// Set stores a value in cache with TTL.
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, c.prefix+key, value, ttl).Err()
}

// Delete removes a key from cache.
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = c.prefix + k
	}
	return c.client.Del(ctx, prefixedKeys...).Err()
}

// DeletePattern removes all keys matching a pattern.
func (c *Client) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, c.prefix+pattern, 0).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

// Client returns the underlying redis client for advanced operations.
func (c *Client) Redis() *redis.Client {
	return c.client
}
