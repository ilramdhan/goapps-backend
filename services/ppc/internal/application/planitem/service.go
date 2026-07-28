// Package planitem provides Layer-2 plan-item application usecases.
package planitem

import (
	"context"
	"time"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// ProductValidator asserts a finance cost-product-master sys id exists + active.
// Nil-safe: a nil validator skips validation (degraded mode).
type ProductValidator interface {
	ValidateProduct(ctx context.Context, sysID int64) error
}

// ProductLookup resolves product code/name for display. Implemented by the
// finance gRPC client; may be nil (names degrade to empty).
type ProductLookup interface {
	BatchGetProducts(ctx context.Context, sysIDs []int64) ([]*financev1.CostMasterProduct, error)
}

// DemandLinkChecker reports whether a demand has a finance product linked yet.
// Kept as a narrow port so plan-item planning does not depend on the whole
// demand aggregate. Nil-safe: a nil checker skips the guard.
type DemandLinkChecker interface {
	DemandProductLinked(ctx context.Context, demandID int64) (bool, error)
}

// Service bundles plan-item usecases over the plan-item repository.
type Service struct {
	repo          planitemdomain.Repository
	validator     ProductValidator
	products      ProductLookup
	capacity      CapacityProvider
	routes        RouteProvider
	machineGroups MachineGroupResolver
	demandLinks   DemandLinkChecker
}

// WithDemandLinks attaches the demand product-link guard. Without one, plan
// items can be created from a demand that has no product yet.
func (s *Service) WithDemandLinks(c DemandLinkChecker) *Service {
	s.demandLinks = c
	return s
}

// NewService creates a plan-item application service. A nil validator disables
// product validation and a nil products lookup disables name resolution
// (both graceful degradation).
func NewService(repo planitemdomain.Repository, validator ProductValidator, products ProductLookup) *Service {
	return &Service{repo: repo, validator: validator, products: products}
}

// WithCapacity attaches a daily-capacity provider used to derive planned
// durations. Without one every derived duration falls back to one day.
func (s *Service) WithCapacity(c CapacityProvider) *Service {
	s.capacity = c
	return s
}

// LookupProducts batch-resolves product code/name for the given sys ids, keyed
// by sys id. Nil-safe: returns an empty map when no lookup is wired, the id
// list is empty, or the lookup call fails.
func (s *Service) LookupProducts(ctx context.Context, sysIDs []int64) map[int64]*financev1.CostMasterProduct {
	byID := make(map[int64]*financev1.CostMasterProduct)
	if s.products == nil || len(sysIDs) == 0 {
		return byID
	}
	products, err := s.products.BatchGetProducts(ctx, sysIDs)
	if err != nil {
		return byID
	}
	for _, p := range products {
		byID[p.GetProductSysId()] = p
	}
	return byID
}

// resolveShade returns the shade code/name of a product from the finance
// master. Shade is descriptive: an unresolvable product yields empty strings
// rather than an error.
func (s *Service) resolveShade(ctx context.Context, sysID int64) (code, name string) {
	p := s.LookupProducts(ctx, []int64{sysID})[sysID]
	if p == nil {
		return "", ""
	}
	return p.GetShadeCode(), p.GetShadeName()
}

// CreateCommand carries inputs for creating a plan item.
type CreateCommand struct {
	CpmProductSysID    int64
	Type               string
	DemandID           *int64
	ParentItemID       *int64
	QtyTarget          float64
	Deadline           time.Time
	RMSource           string
	MachineGroupID     int64
	PreferredMachineID *int64
	Month              string
	MonthOverride      bool
	Timeline           planitemdomain.TimelineParams
	Notes              string
	CreatedBy          int64
}

// CreateResult carries the created FG item plus every cascade-generated
// upstream item, ordered FG-first then by increasing route level.
type CreateResult struct {
	Item     *planitemdomain.PlanItem
	Children []*planitemdomain.PlanItem
	// Warning is set when the cascade could not run but creation legitimately
	// succeeded — most often a product with no released route. Never an error:
	// a product without a route must still be plannable.
	Warning string
	// RouteLookups counts the finance route RPCs the cascade issued (one per
	// hop, serially — PPC and finance are separate databases).
	RouteLookups int
}

// Create validates the product ref then persists a plan item. When an
// FG_DELIVERY item is created, its released route is walked upstream and one
// INTERMEDIATE item is generated per level — each with its own machine group,
// its own product's shade and its own back-dated deadline. The FG and the whole
// chain are written in a single transaction: a partial cascade is worse than
// none.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*CreateResult, error) {
	if err := s.ensureDemandLinked(ctx, cmd.DemandID); err != nil {
		return nil, err
	}
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}
	shadeCode, shadeName := s.resolveShade(ctx, cmd.CpmProductSysID)
	entity, err := planitemdomain.New(planitemdomain.NewParams{
		CpmProductSysID:    cmd.CpmProductSysID,
		Type:               cmd.Type,
		DemandID:           cmd.DemandID,
		ParentItemID:       cmd.ParentItemID,
		QtyTarget:          cmd.QtyTarget,
		Deadline:           cmd.Deadline,
		RMSource:           cmd.RMSource,
		ShadeCode:          shadeCode,
		ShadeName:          shadeName,
		MachineGroupID:     cmd.MachineGroupID,
		PreferredMachineID: cmd.PreferredMachineID,
		Month:              cmd.Month,
		MonthOverride:      cmd.MonthOverride,
		Timeline:           cmd.Timeline,
		Notes:              cmd.Notes,
		CreatedBy:          cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	s.applyDerivedTimeline(ctx, entity)

	if cmd.Type != planitemdomain.TypeFGDelivery {
		if err := s.repo.Create(ctx, entity); err != nil {
			return nil, err
		}
		return &CreateResult{Item: entity}, nil
	}

	cascade, err := s.cascadeRoute(ctx, entity)
	if err != nil {
		return nil, err
	}
	batch := append([]*planitemdomain.PlanItem{entity}, cascade.items...)
	if err := s.repo.CreateBatch(ctx, batch); err != nil {
		return nil, err
	}
	return &CreateResult{
		Item:         entity,
		Children:     cascade.items,
		Warning:      cascade.warning,
		RouteLookups: cascade.rpcCalls,
	}, nil
}

// ensureDemandLinked refuses planning against a demand whose finance product is
// still unlinked. Committing machines and materials to an unknown product is
// worse than refusing the plan item outright. Nil-safe on both the checker and
// the demand id (a cascade child carries a parent, not a demand).
func (s *Service) ensureDemandLinked(ctx context.Context, demandID *int64) error {
	if s.demandLinks == nil || demandID == nil {
		return nil
	}
	linked, err := s.demandLinks.DemandProductLinked(ctx, *demandID)
	if err != nil {
		return err
	}
	if !linked {
		return planitemdomain.ErrDemandProductNotLinked
	}
	return nil
}

// Get retrieves a plan item by ID.
func (s *Service) Get(ctx context.Context, id int64) (*planitemdomain.PlanItem, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries editable plan-item fields plus a change reason.
type UpdateCommand struct {
	ID                 int64
	QtyTarget          *float64
	Deadline           *time.Time
	RMSource           *string
	Sequence           *int32
	Status             *string
	MachineGroupID     *int64
	PreferredMachineID *int64
	Timeline           planitemdomain.TimelineParams
	Notes              *string
	ChangeReason       string
	ChangedBy          int64
}

// Update mutates a plan item and records field-level changes in the plan log.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*planitemdomain.PlanItem, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	changes, err := entity.Update(planitemdomain.UpdateParams{
		QtyTarget:          cmd.QtyTarget,
		Deadline:           cmd.Deadline,
		RMSource:           cmd.RMSource,
		Sequence:           cmd.Sequence,
		Status:             cmd.Status,
		MachineGroupID:     cmd.MachineGroupID,
		PreferredMachineID: cmd.PreferredMachineID,
		Timeline:           cmd.Timeline,
		Notes:              cmd.Notes,
	})
	if err != nil {
		return nil, err
	}
	// Recompute only when the item is still system-derived: a MANUAL override
	// must survive later quantity edits untouched.
	if cmd.QtyTarget != nil {
		s.applyDerivedTimeline(ctx, entity)
	}
	log := make([]planitemdomain.LogEntry, 0, len(changes))
	for _, c := range changes {
		log = append(log, planitemdomain.LogEntry{
			Field:     c.Field,
			Before:    c.Before,
			After:     c.After,
			ChangedBy: cmd.ChangedBy,
			Reason:    cmd.ChangeReason,
		})
	}
	if err := s.repo.Update(ctx, entity, log); err != nil {
		return nil, err
	}
	return entity, nil
}

// Delete removes a plan item by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing plan items.
type ListQuery struct {
	Page           int
	PageSize       int
	Search         string
	Month          string
	Type           string
	Status         string
	MachineGroupID *int64
	DemandID       *int64
	SortBy         string
	SortOrder      string
}

// ListResult holds a page of plan items plus pagination metadata.
type ListResult struct {
	Items       []*planitemdomain.PlanItem
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of plan items.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := planitemdomain.ListFilter{
		Search:         q.Search,
		Month:          q.Month,
		Type:           q.Type,
		Status:         q.Status,
		MachineGroupID: q.MachineGroupID,
		DemandID:       q.DemandID,
		Page:           q.Page,
		PageSize:       q.PageSize,
		SortBy:         q.SortBy,
		SortOrder:      q.SortOrder,
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

// GanttQuery scopes a Gantt-view query.
type GanttQuery struct {
	Month          string
	Area           string
	MachineGroupID *int64
	FromDate       *time.Time
	ToDate         *time.Time
}

// GetGanttView returns plan items for the timeline (Y=machine group, X=days).
// The delivery layer shapes these into bars; this returns the underlying data.
func (s *Service) GetGanttView(ctx context.Context, q GanttQuery) ([]*planitemdomain.GanttRow, error) {
	return s.repo.ListForGantt(ctx, planitemdomain.GanttFilter{
		Month:          q.Month,
		Area:           q.Area,
		MachineGroupID: q.MachineGroupID,
		FromDate:       q.FromDate,
		ToDate:         q.ToDate,
	})
}
