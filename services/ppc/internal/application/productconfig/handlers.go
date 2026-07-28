// Package productconfig provides application-layer handlers for product-config CRUD.
package productconfig

import (
	"context"

	configdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/productconfig"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// ProductValidator asserts that a finance cost-product-master sys id exists and
// is active. Implemented by the financeclient; nil-safe so the service runs
// without finance (degraded mode).
type ProductValidator interface {
	ValidateProduct(ctx context.Context, sysID int64) error
}

// Service bundles all product-config use cases over a single repository.
type Service struct {
	repo      configdomain.Repository
	validator ProductValidator
}

// NewService creates a new product-config application service. A nil validator
// disables product validation (graceful degradation).
func NewService(repo configdomain.Repository, validator ProductValidator) *Service {
	return &Service{repo: repo, validator: validator}
}

// CreateCommand carries inputs for creating a product config.
type CreateCommand struct {
	CpmProductSysID  int64
	IsCommodityWatch bool
	PriceSell        *float64
	MachineGroupID   *int64
	YieldStd         *float64
	BufferRmPct      *float64
	AxYieldPct       *float64
	Denier           *float64
	CreatedBy        string
}

// Create validates and persists a new product config. The referenced
// cost-product-master sys id is validated against finance before persistence.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*configdomain.ProductConfig, error) {
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}

	entity, err := configdomain.NewProductConfig(
		cmd.CpmProductSysID,
		cmd.IsCommodityWatch,
		cmd.PriceSell,
		cmd.MachineGroupID,
		cmd.YieldStd,
		cmd.BufferRmPct,
		cmd.AxYieldPct,
		cmd.Denier,
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

// Get retrieves a product config by ID.
func (s *Service) Get(ctx context.Context, id int64) (*configdomain.ProductConfig, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a product config.
type UpdateCommand struct {
	ID               int64
	IsCommodityWatch *bool
	PriceSell        *float64
	MachineGroupID   *int64
	YieldStd         *float64
	BufferRmPct      *float64
	AxYieldPct       *float64
	Denier           *float64
	UpdatedBy        string
}

// Update mutates an existing product config.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*configdomain.ProductConfig, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(
		cmd.IsCommodityWatch,
		cmd.PriceSell,
		cmd.MachineGroupID,
		cmd.YieldStd,
		cmd.BufferRmPct,
		cmd.AxYieldPct,
		cmd.Denier,
		cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a product config by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing product configs.
type ListQuery struct {
	Page               int
	PageSize           int
	Search             string
	CommodityWatchOnly *bool
	SortBy             string
	SortOrder          string
}

// ListResult holds a page of product configs plus pagination metadata.
type ListResult struct {
	Items       []*configdomain.ProductConfig
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of product configs.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := configdomain.ListFilter{
		Search:             query.Search,
		CommodityWatchOnly: query.CommodityWatchOnly,
		Page:               query.Page,
		PageSize:           query.PageSize,
		SortBy:             query.SortBy,
		SortOrder:          query.SortOrder,
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
