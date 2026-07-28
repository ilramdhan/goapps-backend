// Package threshold provides application-layer handlers for overrun-threshold-config CRUD.
package threshold

import (
	"context"

	thresholddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/threshold"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all overrun-threshold-config use cases over a single repository.
type Service struct {
	repo thresholddomain.Repository
}

// NewService creates a new overrun-threshold-config application service.
func NewService(repo thresholddomain.Repository) *Service {
	return &Service{repo: repo}
}

// CreateCommand carries inputs for creating a config.
type CreateCommand struct {
	Level        string
	RefID        *int64
	Unit         string
	WarningValue float64
	BlockValue   float64
	Notes        string
	CreatedBy    string
}

// Create validates and persists a new config.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*thresholddomain.Config, error) {
	entity, err := thresholddomain.NewConfig(
		cmd.Level, cmd.RefID, cmd.Unit, cmd.WarningValue, cmd.BlockValue, cmd.Notes, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a config by ID.
func (s *Service) Get(ctx context.Context, id int64) (*thresholddomain.Config, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a config.
type UpdateCommand struct {
	ID           int64
	Unit         *string
	WarningValue *float64
	BlockValue   *float64
	Notes        *string
	IsActive     *bool
	UpdatedBy    string
}

// Update mutates an existing config.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*thresholddomain.Config, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.Unit, cmd.WarningValue, cmd.BlockValue, cmd.Notes, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a config by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing configs.
type ListQuery struct {
	Page      int
	PageSize  int
	Level     string
	IsActive  *bool
	SortBy    string
	SortOrder string
}

// ListResult holds a page of configs plus pagination metadata.
type ListResult struct {
	Items       []*thresholddomain.Config
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of configs.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := thresholddomain.ListFilter{
		Level:     query.Level,
		IsActive:  query.IsActive,
		Page:      query.Page,
		PageSize:  query.PageSize,
		SortBy:    query.SortBy,
		SortOrder: query.SortOrder,
	}
	filter.Validate()

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if filter.PageSize > 0 && total > 0 {
		totalPages = safeconv.Int64ToInt32((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}
