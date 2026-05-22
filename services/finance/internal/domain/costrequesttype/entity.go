// Package costrequesttype is the CostRequestType domain (PRD Phase A §7.1.3, CRT_).
package costrequesttype

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a request type is missing.
var ErrNotFound = errors.New("cost request type not found")

// CostRequestType is a read-mostly lookup row.
type CostRequestType struct {
	TypeID              int32
	Code                string
	DisplayName         string
	StateMachineVariant string // FULL | SHORTCUT_CAPABLE
	DefaultUrgency      string
	IsActive            bool
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
	List(ctx context.Context, f Filter) (items []*CostRequestType, total int64, err error)
	GetByID(ctx context.Context, id int32) (*CostRequestType, error)
}
