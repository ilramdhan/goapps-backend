// Package datasource contains the BI data source aggregate (registry of where fact data comes from).
package datasource

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors.
var (
	// ErrNotFound is returned when no data source matches a lookup.
	ErrNotFound = errors.New("data source not found")
	// ErrInvalidCode is returned on source_code validation failure.
	ErrInvalidCode = errors.New("invalid data source code")
)

// DataSource is a read-only registry entry. CRUD on this table is super-admin only
// and out of scope for spec 1A+1B (admin form just lists them in a dropdown).
type DataSource struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Type        string // ORACLE|LARAVEL|EXCEL|MANUAL|API
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository is the read contract.
type Repository interface {
	List(ctx context.Context, includeInactive bool) ([]*DataSource, error)
	GetByCode(ctx context.Context, code string) (*DataSource, error)
	GetByID(ctx context.Context, id uuid.UUID) (*DataSource, error)
}
