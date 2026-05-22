// Package costpapertubetype is the CostPaperTubeType domain (PRD Phase A, CPTT_).
package costpapertubetype

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a paper tube type is missing.
var ErrNotFound = errors.New("cost paper tube type not found")

// CostPaperTubeType is a read-mostly lookup row.
type CostPaperTubeType struct {
	PaperTubeTypeID int32
	Code            string
	DisplayName     string
	IsActive        bool
}

// Filter for List.
type Filter struct {
	Search       string
	ActiveFilter string
	Page         int
	PageSize     int
}

// Repository exposes read access.
type Repository interface {
	List(ctx context.Context, f Filter) (items []*CostPaperTubeType, total int64, err error)
	GetByID(ctx context.Context, id int32) (*CostPaperTubeType, error)
}
