// Package redis — chart_cache.go provides a typed Redis read-through cache for BI chart data.
package redis

import (
	"context"
	"crypto/md5" //nolint:gosec // not for security; just for fingerprinting cache keys
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ChartCache is a typed wrapper around Client for storing chart data payloads
// keyed by `{dashboard_code}:{md5(filters_json)}`.
//
// The underlying Client.prefix already namespaces all keys (per-service); this
// layer adds the `bi:chart:` sub-prefix.
type ChartCache struct {
	c *Client
}

// NewChartCache constructs a ChartCache wrapping the given Client.
func NewChartCache(c *Client) *ChartCache {
	return &ChartCache{c: c}
}

const chartKeyPrefix = "bi:chart:"

// Get reads the cached payload for (dashboardCode, filtersHash) into out.
//
// Returns hit=false when the key is missing. Other errors propagate as-is.
func (cc *ChartCache) Get(ctx context.Context, dashboardCode, filtersHash string, out any) (bool, error) {
	if cc == nil || cc.c == nil {
		return false, nil
	}
	raw, err := cc.c.Get(ctx, keyFor(dashboardCode, filtersHash))
	if errors.Is(err, goredis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("chart cache get: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false, fmt.Errorf("chart cache decode: %w", err)
	}
	return true, nil
}

// Set writes the payload with the given TTL. TTL <= 0 is a no-op.
func (cc *ChartCache) Set(ctx context.Context, dashboardCode, filtersHash string, value any, ttl time.Duration) error {
	if cc == nil || cc.c == nil || ttl <= 0 {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("chart cache encode: %w", err)
	}
	if err := cc.c.Set(ctx, keyFor(dashboardCode, filtersHash), raw, ttl); err != nil {
		return fmt.Errorf("chart cache set: %w", err)
	}
	return nil
}

// InvalidateDashboard removes every cached entry for one dashboard code.
func (cc *ChartCache) InvalidateDashboard(ctx context.Context, dashboardCode string) error {
	if cc == nil || cc.c == nil {
		return nil
	}
	pattern := chartKeyPrefix + dashboardCode + ":*"
	if err := cc.c.DeletePattern(ctx, pattern); err != nil {
		return fmt.Errorf("chart cache invalidate %s: %w", dashboardCode, err)
	}
	return nil
}

// InvalidateAll removes every cached chart entry. Used by ETL or Excel commit
// when a wholesale type-scoped refresh is preferred over per-code targeting.
func (cc *ChartCache) InvalidateAll(ctx context.Context) error {
	if cc == nil || cc.c == nil {
		return nil
	}
	return cc.c.DeletePattern(ctx, chartKeyPrefix+"*")
}

// InvalidateMany removes cached entries for a batch of dashboard codes.
func (cc *ChartCache) InvalidateMany(ctx context.Context, dashboardCodes []string) error {
	for _, code := range dashboardCodes {
		if err := cc.InvalidateDashboard(ctx, code); err != nil {
			return err
		}
	}
	return nil
}

// keyFor builds the canonical cache key.
func keyFor(dashboardCode, filtersHash string) string {
	return chartKeyPrefix + dashboardCode + ":" + filtersHash
}

// HashFilters returns a stable fingerprint for any JSON-serializable filter value.
//
// MD5 is used purely as a fixed-length identity for cache keys; cryptographic
// strength is not required and not implied.
func HashFilters(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "_invalid_"
	}
	sum := md5.Sum(raw) //nolint:gosec // not security-sensitive
	return hex.EncodeToString(sum[:])
}
