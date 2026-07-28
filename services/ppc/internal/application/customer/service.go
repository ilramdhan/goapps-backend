// Package customer provides application-layer use cases for the PPC customer master.
package customer

import (
	"context"

	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles the customer use cases over a single repository.
type Service struct {
	repo customerdomain.Repository
	sync *SyncUsecase
}

// NewService creates a new customer application service.
func NewService(repo customerdomain.Repository) *Service {
	return &Service{repo: repo}
}

// WithSync attaches the Oracle sync usecase. A nil sync leaves the service fully
// usable for CRUD; only SyncFromOracle reports that it is unconfigured.
func (s *Service) WithSync(sync *SyncUsecase) *Service {
	s.sync = sync
	return s
}

// CreateCommand carries inputs for creating a customer by hand.
type CreateCommand struct {
	Code       string
	Name       string
	ShortName  *string
	TaxNo      *string
	ParentCode *string
	CreatedBy  string
}

// Create validates and persists a hand-authored customer.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*customerdomain.Customer, error) {
	entity, err := customerdomain.New(customerdomain.NewParams(cmd))
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a customer by ID.
func (s *Service) Get(ctx context.Context, id int64) (*customerdomain.Customer, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries inputs for updating a customer. The code is immutable —
// it is the key the Oracle sync upserts on.
type UpdateCommand struct {
	ID         int64
	Name       *string
	ShortName  *string
	TaxNo      *string
	ParentCode *string
	IsActive   *bool
	UpdatedBy  string
}

// Update mutates an existing customer.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*customerdomain.Customer, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(customerdomain.UpdateParams{
		Name:       cmd.Name,
		ShortName:  cmd.ShortName,
		TaxNo:      cmd.TaxNo,
		ParentCode: cmd.ParentCode,
		IsActive:   cmd.IsActive,
		UpdatedBy:  cmd.UpdatedBy,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// ListQuery carries inputs for listing customers.
type ListQuery struct {
	Search    string
	IsActive  *bool
	Source    string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

// ListResult holds a page of customers plus pagination metadata.
type ListResult struct {
	Items       []*customerdomain.Customer
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of customers.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := customerdomain.ListFilter{
		Search:    query.Search,
		IsActive:  query.IsActive,
		Source:    query.Source,
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

// SyncFromOracle runs the Oracle customer ETL on demand (the "Sync from Oracle"
// button). Reports ErrSyncNotConfigured when Oracle was never wired.
func (s *Service) SyncFromOracle(ctx context.Context) (SyncResult, error) {
	if s.sync == nil {
		return SyncResult{}, customerdomain.ErrSyncNotConfigured
	}
	return s.sync.Sync(ctx)
}

// ResolveCodes maps customer codes to master ids. Used by the demand pull to link
// SO-staging rows to the customer master.
func (s *Service) ResolveCodes(ctx context.Context, codes []string) (map[string]int64, error) {
	return s.repo.ResolveCodes(ctx, codes)
}
