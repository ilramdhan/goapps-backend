// Package productmachineparameter provides application-layer handlers for
// product-machine-parameter CRUD.
package productmachineparameter

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/productmachineparameter"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// ProductValidator asserts that a finance cost-product-master sys id exists and
// is active. Implemented by the financeclient; nil-safe so the service runs
// without finance (degraded mode).
type ProductValidator interface {
	ValidateProduct(ctx context.Context, sysID int64) error
}

// Service bundles all product-machine-parameter use cases over a single repository.
type Service struct {
	repo      domain.Repository
	validator ProductValidator
}

// NewService creates a new product-machine-parameter application service. A nil
// validator disables product validation (graceful degradation).
func NewService(repo domain.Repository, validator ProductValidator) *Service {
	return &Service{repo: repo, validator: validator}
}

// CreateCommand carries inputs for creating a parameter value.
type CreateCommand struct {
	CpmProductSysID int64
	MachineID       int64
	ParamID         string
	ValueNum        *float64
	ValueText       *string
	ValueFlag       *bool
}

// Create validates and persists a new parameter value. The referenced
// cost-product-master sys id is validated against finance before persistence.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Parameter, error) {
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}

	entity, err := domain.NewParameter(
		cmd.CpmProductSysID,
		cmd.MachineID,
		cmd.ParamID,
		cmd.ValueNum,
		cmd.ValueText,
		cmd.ValueFlag,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a parameter value by ID.
func (s *Service) Get(ctx context.Context, id int64) (*domain.Parameter, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a parameter value.
type UpdateCommand struct {
	ID        int64
	ValueNum  *float64
	ValueText *string
	ValueFlag *bool
}

// Update mutates the typed value of an existing parameter.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Parameter, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	entity.Update(cmd.ValueNum, cmd.ValueText, cmd.ValueFlag)
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a parameter value by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing parameter values.
type ListQuery struct {
	Page            int
	PageSize        int
	CpmProductSysID *int64
	MachineID       *int64
	ParamID         string
	SortBy          string
	SortOrder       string
}

// ListResult holds a page of parameter values plus pagination metadata.
type ListResult struct {
	Items       []*domain.Parameter
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of parameter values.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := domain.ListFilter{
		CpmProductSysID: query.CpmProductSysID,
		MachineID:       query.MachineID,
		ParamID:         query.ParamID,
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
