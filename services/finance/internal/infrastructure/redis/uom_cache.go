// Package redis provides Redis connection and caching utilities.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
)

const (
	// UOM cache keys
	uomByIDKey   = "uom:id:%s"
	uomByCodeKey = "uom:code:%s"
	uomListKey   = "uom:list:%s"

	// Default TTL
	defaultTTL = 15 * time.Minute
	listTTL    = 5 * time.Minute
)

// UOMCache provides caching for UOM data.
type UOMCache struct {
	client *Client
}

// NewUOMCache creates a new UOM cache.
func NewUOMCache(client *Client) *UOMCache {
	return &UOMCache{client: client}
}

// uomCacheData is the cached representation of UOM.
type uomCacheData struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	IsActive    bool    `json:"is_active"`
	CreatedAt   string  `json:"created_at"`
	CreatedBy   string  `json:"created_by"`
	UpdatedAt   *string `json:"updated_at,omitempty"`
	UpdatedBy   *string `json:"updated_by,omitempty"`
}

// GetByID retrieves a UOM by ID from cache.
func (c *UOMCache) GetByID(ctx context.Context, id uuid.UUID) (*uom.UOM, error) {
	key := fmt.Sprintf(uomByIDKey, id.String())
	data, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, err // Cache miss
	}

	var cached uomCacheData
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		log.Warn().Err(err).Str("key", key).Msg("Failed to unmarshal cached UOM")
		return nil, err
	}

	return c.toEntity(&cached)
}

// SetByID caches a UOM by ID.
func (c *UOMCache) SetByID(ctx context.Context, entity *uom.UOM) error {
	key := fmt.Sprintf(uomByIDKey, entity.ID().String())
	data := c.fromEntity(entity)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, string(jsonData), defaultTTL)
}

// GetByCode retrieves a UOM by code from cache.
func (c *UOMCache) GetByCode(ctx context.Context, code string) (*uom.UOM, error) {
	key := fmt.Sprintf(uomByCodeKey, code)
	data, err := c.client.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var cached uomCacheData
	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return nil, err
	}

	return c.toEntity(&cached)
}

// SetByCode caches a UOM by code.
func (c *UOMCache) SetByCode(ctx context.Context, entity *uom.UOM) error {
	key := fmt.Sprintf(uomByCodeKey, entity.Code().String())
	data := c.fromEntity(entity)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, string(jsonData), defaultTTL)
}

// InvalidateByID removes a UOM from cache by ID.
func (c *UOMCache) InvalidateByID(ctx context.Context, id uuid.UUID) error {
	return c.client.Delete(ctx, fmt.Sprintf(uomByIDKey, id.String()))
}

// InvalidateByCode removes a UOM from cache by code.
func (c *UOMCache) InvalidateByCode(ctx context.Context, code string) error {
	return c.client.Delete(ctx, fmt.Sprintf(uomByCodeKey, code))
}

// InvalidateAll removes all UOM cache entries.
func (c *UOMCache) InvalidateAll(ctx context.Context) error {
	if err := c.client.DeletePattern(ctx, "uom:*"); err != nil {
		log.Warn().Err(err).Msg("Failed to invalidate UOM cache")
		return err
	}
	return nil
}

// InvalidateList removes list cache entries.
func (c *UOMCache) InvalidateList(ctx context.Context) error {
	return c.client.DeletePattern(ctx, "uom:list:*")
}

// fromEntity converts domain entity to cache data.
func (c *UOMCache) fromEntity(entity *uom.UOM) *uomCacheData {
	data := &uomCacheData{
		ID:          entity.ID().String(),
		Code:        entity.Code().String(),
		Name:        entity.Name(),
		Category:    entity.Category().String(),
		Description: entity.Description(),
		IsActive:    entity.IsActive(),
		CreatedAt:   entity.CreatedAt().Format(time.RFC3339),
		CreatedBy:   entity.CreatedBy(),
	}

	if entity.UpdatedAt() != nil {
		updatedAt := entity.UpdatedAt().Format(time.RFC3339)
		data.UpdatedAt = &updatedAt
	}
	if entity.UpdatedBy() != nil {
		data.UpdatedBy = entity.UpdatedBy()
	}

	return data
}

// toEntity converts cache data to domain entity.
func (c *UOMCache) toEntity(data *uomCacheData) (*uom.UOM, error) {
	id, err := uuid.Parse(data.ID)
	if err != nil {
		return nil, err
	}

	code, err := uom.NewCode(data.Code)
	if err != nil {
		return nil, err
	}

	category, err := uom.NewCategory(data.Category)
	if err != nil {
		return nil, err
	}

	createdAt, err := time.Parse(time.RFC3339, data.CreatedAt)
	if err != nil {
		return nil, err
	}

	var updatedAt *time.Time
	if data.UpdatedAt != nil {
		t, err := time.Parse(time.RFC3339, *data.UpdatedAt)
		if err == nil {
			updatedAt = &t
		}
	}

	return uom.ReconstructUOM(
		id,
		code,
		data.Name,
		category,
		data.Description,
		data.IsActive,
		createdAt,
		data.CreatedBy,
		updatedAt,
		data.UpdatedBy,
		nil, // deletedAt
		nil, // deletedBy
	), nil
}
