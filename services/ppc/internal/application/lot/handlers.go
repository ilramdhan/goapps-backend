// Package lot provides application-layer handlers for lot-master CRUD.
package lot

import (
	"context"

	lotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// Service bundles all lot-master use cases over a single repository.
type Service struct {
	repo lotdomain.Repository
	sync *SyncUsecase
}

// NewService creates a new lot-master application service.
func NewService(repo lotdomain.Repository) *Service {
	return &Service{repo: repo}
}

// WithSync attaches the Oracle MMSMERGE sync usecase. A nil sync leaves the
// service fully usable for CRUD; only SyncFromOracle reports it unconfigured.
func (s *Service) WithSync(sync *SyncUsecase) *Service {
	s.sync = sync
	return s
}

// SyncFromOracle runs the legacy lot-master import on demand (the "Sync from
// Oracle" button). Reports ErrSyncNotConfigured when Oracle was never wired.
func (s *Service) SyncFromOracle(ctx context.Context) (SyncResult, error) {
	if s.sync == nil {
		return SyncResult{}, lotdomain.ErrSyncNotConfigured
	}
	return s.sync.Sync(ctx)
}

// CreateCommand carries inputs for creating a lot master.
type CreateCommand struct {
	LotNo           string
	ItemCode        string
	ShadeCode       string
	StdWeightFull   float64
	StdWeightUnfull float64
	Notes           string
	CreatedBy       string
}

// Create validates and persists a new lot master.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*lotdomain.Master, error) {
	entity, err := lotdomain.NewMaster(
		cmd.LotNo, cmd.ItemCode, cmd.ShadeCode,
		cmd.StdWeightFull, cmd.StdWeightUnfull, cmd.Notes, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Get retrieves a lot master by lot number.
func (s *Service) Get(ctx context.Context, lotNo string) (*lotdomain.Master, error) {
	return s.repo.GetByID(ctx, lotNo)
}

// UpdateCommand carries inputs for updating a lot master.
type UpdateCommand struct {
	LotNo           string
	ItemCode        *string
	ShadeCode       *string
	StdWeightFull   *float64
	StdWeightUnfull *float64
	Notes           *string
	// Spec, when non-nil, replaces the yarn/packing specification wholesale.
	// Nil leaves the stored spec untouched, so a caller that only edits weights
	// does not have to round-trip twenty fields it does not care about.
	Spec      *lotdomain.Spec
	UpdatedBy string
}

// Update mutates an existing lot master.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*lotdomain.Master, error) {
	entity, err := s.repo.GetByID(ctx, cmd.LotNo)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(cmd.ItemCode, cmd.ShadeCode, cmd.StdWeightFull, cmd.StdWeightUnfull, cmd.Notes, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if cmd.Spec != nil {
		entity.UpdateSpec(*cmd.Spec, cmd.UpdatedBy)
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a lot master by lot number.
func (s *Service) Delete(ctx context.Context, lotNo string) error {
	return s.repo.Delete(ctx, lotNo)
}

// ListQuery carries inputs for listing lot masters.
type ListQuery struct {
	Page      int
	PageSize  int
	Search    string
	ItemCode  string
	ShadeCode string
	// Source filters on provenance (lotdomain.SourcePPC / SourceMMSMERGE).
	Source string
	// ProdType filters on the MMSMERGE product type (PTY/POY/FOY).
	ProdType  string
	SortBy    string
	SortOrder string
}

// ListResult holds a page of lot masters plus pagination metadata.
type ListResult struct {
	Items       []*lotdomain.Master
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of lot masters.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListResult, error) {
	filter := lotdomain.ListFilter{
		Search:    query.Search,
		ItemCode:  query.ItemCode,
		ShadeCode: query.ShadeCode,
		Source:    query.Source,
		ProdType:  query.ProdType,
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
