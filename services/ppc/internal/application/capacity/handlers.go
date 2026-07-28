// Package capacity provides application-layer handlers for product-machine-capacity CRUD.
package capacity

import (
	"context"

	capacitydomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/capacity"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// ProductValidator asserts that a finance cost-product-master sys id exists and
// is active. Implemented by the financeclient; nil-safe so the service runs
// without finance (degraded mode).
type ProductValidator interface {
	ValidateProduct(ctx context.Context, sysID int64) error
}

// Service bundles all product-machine-capacity use cases over a single repository.
type Service struct {
	repo      capacitydomain.Repository
	validator ProductValidator
}

// NewService creates a new product-machine-capacity application service. A nil
// validator disables product validation (graceful degradation).
func NewService(repo capacitydomain.Repository, validator ProductValidator) *Service {
	return &Service{repo: repo, validator: validator}
}

// CreateCommand carries inputs for creating a capacity.
type CreateCommand struct {
	CpmProductSysID int64
	MachineID       int64
	ProdPerDay      *float64
	EfficiencyPct   *float64
	CreatedBy       string
}

// Create validates and persists a new capacity. The referenced
// cost-product-master sys id is validated against finance before persistence.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*capacitydomain.Capacity, error) {
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}

	entity, err := capacitydomain.NewCapacity(
		cmd.CpmProductSysID,
		cmd.MachineID,
		cmd.ProdPerDay,
		cmd.EfficiencyPct,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a capacity by ID.
func (s *Service) Get(ctx context.Context, id int64) (*capacitydomain.Capacity, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a capacity.
type UpdateCommand struct {
	ID            int64
	ProdPerDay    *float64
	EfficiencyPct *float64
	UpdatedBy     string
}

// Update mutates an existing capacity.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*capacitydomain.Capacity, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(
		cmd.ProdPerDay,
		cmd.EfficiencyPct,
		cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a capacity by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing capacities.
type ListQuery struct {
	Page            int
	PageSize        int
	CpmProductSysID *int64
	MachineID       *int64
	SortBy          string
	SortOrder       string
}

// ListResult holds a page of capacities plus pagination metadata.
type ListResult struct {
	Items       []*capacitydomain.Capacity
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of capacities.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := capacitydomain.ListFilter{
		CpmProductSysID: query.CpmProductSysID,
		MachineID:       query.MachineID,
		Page:            query.Page,
		PageSize:        query.PageSize,
		SortBy:          query.SortBy,
		SortOrder:       query.SortOrder,
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
