// Package costerp contains read-only domain types for ERP lookup tables
// (cost_erp_item, cost_erp_grade, cost_erp_shade — PRD Phase B §7.3).
package costerp

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when an ERP lookup row is not found.
var ErrNotFound = errors.New("erp lookup record not found")

// Item is a read-only ERP master_item row.
type Item struct {
	ItemID   int64
	ItemCode string
	ItemName string
	ItemType string
	IsActive bool
	SyncedAt time.Time
}

// Grade is a read-only ERP grade row.
type Grade struct {
	GradeID   int32
	GradeCode string
	GradeName string
	IsActive  bool
	SyncedAt  time.Time
}

// Shade is a read-only ERP shade row.
type Shade struct {
	ShadeID   int32
	ShadeCode string
	ShadeName string
	IsActive  bool
	SyncedAt  time.Time
}

// ItemFilter for ListItems.
type ItemFilter struct {
	Search       string
	ItemType     string
	ActiveFilter string
	Page         int
	PageSize     int
}

// LookupFilter for ListGrades/ListShades.
type LookupFilter struct {
	Search       string
	ActiveFilter string
	Page         int
	PageSize     int
}

// Repository exposes read-only ERP lookups.
type Repository interface {
	ListItems(ctx context.Context, f ItemFilter) (items []*Item, total int64, err error)
	GetItem(ctx context.Context, itemID int64) (*Item, error)
	ListGrades(ctx context.Context, f LookupFilter) (items []*Grade, total int64, err error)
	ListShades(ctx context.Context, f LookupFilter) (items []*Shade, total int64, err error)
}
