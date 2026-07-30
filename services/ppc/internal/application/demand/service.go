// Package demand provides Layer-1 demand application usecases.
package demand

import (
	"context"
	"time"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/notification"
	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
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

// LookupOrionItemCodes resolves the Orion item code behind each given demand,
// keyed by demand id, by following its sos_ref into the staging inbox. Demands
// not pulled from Orion are simply absent from the map.
//
// This is display decoration, so it degrades rather than fails: a lookup error
// yields an empty map and the caller renders without the code.
func (s *Service) LookupOrionItemCodes(ctx context.Context, items []*demanddomain.Demand) map[int64]string {
	byDemand := make(map[int64]string, len(items))
	sosIDs := make([]int64, 0, len(items))
	sosToDemand := make(map[int64][]int64, len(items))
	for _, e := range items {
		ref := e.SosRef()
		if ref == nil || *ref == 0 {
			continue
		}
		if _, seen := sosToDemand[*ref]; !seen {
			sosIDs = append(sosIDs, *ref)
		}
		sosToDemand[*ref] = append(sosToDemand[*ref], e.ID())
	}
	if len(sosIDs) == 0 {
		return byDemand
	}
	codes, err := s.repo.LookupStagingItemCodes(ctx, sosIDs)
	if err != nil {
		return byDemand
	}
	for sosID, code := range codes {
		for _, demandID := range sosToDemand[sosID] {
			byDemand[demandID] = code
		}
	}
	return byDemand
}

// CustomerResolver maps Orion customer codes onto PPC customer-master ids.
// Implemented by the customer application service; may be nil (pulled demands
// then carry no customer).
type CustomerResolver interface {
	ResolveCodes(ctx context.Context, codes []string) (map[string]int64, error)
	// ResolveIDs maps customer ids back to code/name, for display decoration of
	// demands that store only the id.
	ResolveIDs(ctx context.Context, ids []int64) (map[int64]customerdomain.Label, error)
}

// LookupCustomers resolves the customer code/name behind each given demand,
// keyed by demand id. Demands with no customer are absent from the map.
//
// Like the product and Orion-code lookups this is display decoration, so it
// degrades rather than fails: no resolver or a lookup error yields an empty map
// and the caller renders the demand without a customer name.
func (s *Service) LookupCustomers(ctx context.Context, items []*demanddomain.Demand) map[int64]customerdomain.Label {
	byDemand := make(map[int64]customerdomain.Label, len(items))
	if s.customers == nil {
		return byDemand
	}
	ids := make([]int64, 0, len(items))
	customerToDemand := make(map[int64][]int64, len(items))
	for _, e := range items {
		cid := e.CustomerID()
		if cid == nil || *cid == 0 {
			continue
		}
		if _, seen := customerToDemand[*cid]; !seen {
			ids = append(ids, *cid)
		}
		customerToDemand[*cid] = append(customerToDemand[*cid], e.ID())
	}
	if len(ids) == 0 {
		return byDemand
	}
	labels, err := s.customers.ResolveIDs(ctx, ids)
	if err != nil {
		return byDemand
	}
	for customerID, label := range labels {
		for _, demandID := range customerToDemand[customerID] {
			byDemand[demandID] = label
		}
	}
	return byDemand
}

// Service bundles demand usecases over the demand repository.
type Service struct {
	repo      demanddomain.Repository
	validator ProductValidator
	notifier  notification.Notifier
	resolver  demanddomain.ProductResolver
	products  ProductLookup
	customers CustomerResolver
}

// NewService creates a demand application service. A nil validator disables
// product validation; a nil notifier disables notifications (both graceful).
// Use WithProductResolution to enable staging product resolution and display
// decoration.
func NewService(repo demanddomain.Repository, validator ProductValidator, notifier notification.Notifier) *Service {
	return &Service{repo: repo, validator: validator, notifier: notifier}
}

// WithProductResolution attaches the finance-backed staging product resolver
// and the display lookup used to decorate resolved staging rows with product
// code/name. Either may be nil, which disables that capability gracefully.
func (s *Service) WithProductResolution(resolver demanddomain.ProductResolver, products ProductLookup) *Service {
	s.resolver = resolver
	s.products = products
	return s
}

// WithCustomerResolution attaches the PPC customer-master resolver used to link
// pulled Orion demands to the customer master. A nil resolver disables the link
// gracefully.
func (s *Service) WithCustomerResolution(customers CustomerResolver) *Service {
	s.customers = customers
	return s
}

// CreateCommand carries inputs for creating a demand.
type CreateCommand struct {
	Type            string
	SubType         string
	Source          string
	CpmProductSysID int64
	QtyOriginal     float64
	Deadline        time.Time
	GradeReq        string
	AxMinPct        *float64
	AmMaxPct        *float64
	SosRef          *int64
	CustomerID      *int64
	ContractNo      string
	ContractDate    *time.Time
	Incoterm        string
	LcStatus        string
	StuffAdvanceNo  string
	Month           string
	MonthOverride   bool
	CreatedBy       int64
}

// Create validates the product ref then persists a new demand. MTS demands
// notify Marketing for approval.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*demanddomain.Demand, error) {
	linkReason, err := manualLinkReason(cmd.Type, cmd.CpmProductSysID)
	if err != nil {
		return nil, err
	}
	// Only validate a product that was actually supplied — a deferred link has
	// nothing to validate yet, by design.
	if s.validator != nil && cmd.CpmProductSysID != 0 {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}
	entity, err := demanddomain.New(demanddomain.NewParams{
		ProductLinkReason: linkReason,
		Type:              cmd.Type,
		SubType:           cmd.SubType,
		Source:            cmd.Source,
		CpmProductSysID:   cmd.CpmProductSysID,
		QtyOriginal:       cmd.QtyOriginal,
		Deadline:          cmd.Deadline,
		GradeReq:          cmd.GradeReq,
		AxMinPct:          cmd.AxMinPct,
		AmMaxPct:          cmd.AmMaxPct,
		SosRef:            cmd.SosRef,
		CustomerID:        cmd.CustomerID,
		ContractNo:        cmd.ContractNo,
		ContractDate:      cmd.ContractDate,
		Incoterm:          cmd.Incoterm,
		LcStatus:          cmd.LcStatus,
		StuffAdvanceNo:    cmd.StuffAdvanceNo,
		Month:             cmd.Month,
		MonthOverride:     cmd.MonthOverride,
		CreatedBy:         cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	if entity.Type() == demanddomain.TypeMTS {
		notification.Notify(ctx, s.notifier, notification.Message{
			Event:      notification.EventMTSRequested,
			Subject:    "MTS demand awaiting approval",
			Recipients: []string{"MARKETING"},
			EntityID:   entity.ID(),
		})
	}
	return entity, nil
}

// manualLinkReason decides the deferred-link reason for a hand-created demand.
// Only MTS and SAMPLE may be raised before their finance master exists, and
// that case is NO_MASTER_YET — "intentionally unresolved", distinct from the
// AUTO_MATCH_FAILED / AMBIGUOUS reasons an Orion pull records. A CONTRACT demand
// always has a product, so omitting it is rejected rather than deferred.
func manualLinkReason(demandType string, sysID int64) (string, error) {
	if sysID != 0 {
		return "", nil
	}
	switch demandType {
	case demanddomain.TypeMTS, demanddomain.TypeSample:
		return demanddomain.LinkReasonNoMasterYet, nil
	default:
		return "", demanddomain.ErrInvalidProduct
	}
}

// Get retrieves a demand by ID.
func (s *Service) Get(ctx context.Context, id int64) (*demanddomain.Demand, error) {
	return s.repo.GetByID(ctx, id)
}

// UpdateCommand carries editable demand fields.
type UpdateCommand struct {
	ID             int64
	QtyOriginal    *float64
	Deadline       *time.Time
	GradeReq       *string
	AxMinPct       *float64
	AmMaxPct       *float64
	ContractNo     *string
	Incoterm       *string
	LcStatus       *string
	StuffAdvanceNo *string
}

// Update mutates an existing demand.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*demanddomain.Demand, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(demanddomain.UpdateParams{
		QtyOriginal:    cmd.QtyOriginal,
		Deadline:       cmd.Deadline,
		GradeReq:       cmd.GradeReq,
		AxMinPct:       cmd.AxMinPct,
		AmMaxPct:       cmd.AmMaxPct,
		ContractNo:     cmd.ContractNo,
		Incoterm:       cmd.Incoterm,
		LcStatus:       cmd.LcStatus,
		StuffAdvanceNo: cmd.StuffAdvanceNo,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// MapProductCommand carries inputs for mapping a product onto an unmapped demand.
type MapProductCommand struct {
	ID              int64
	CpmProductSysID int64
}

// MapProduct validates and maps a product onto a demand that currently has
// none. Rejects re-mapping an already-mapped demand.
func (s *Service) MapProduct(ctx context.Context, cmd MapProductCommand) (*demanddomain.Demand, error) {
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.SetProduct(cmd.CpmProductSysID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// DemandProductLinked reports whether a demand already has a finance product.
// Exposed so plan-item planning can refuse an unlinked demand without pulling
// in the whole demand aggregate.
func (s *Service) DemandProductLinked(ctx context.Context, id int64) (bool, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return entity.IsProductLinked(), nil
}

// Delete removes a demand by ID.
func (s *Service) Delete(ctx context.Context, id int64) error {
	n, err := s.repo.CountPlanItems(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return demanddomain.ErrHasPlanItems
	}
	return s.repo.Delete(ctx, id)
}

// Confirm confirms a pending demand.
func (s *Service) Confirm(ctx context.Context, id, userID int64) (*demanddomain.Demand, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := entity.Confirm(userID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// ApproveMTS applies the Marketing decision to an MTS demand.
func (s *Service) ApproveMTS(ctx context.Context, id int64, approved bool, userID int64) (*demanddomain.Demand, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := entity.ApproveMTS(approved, userID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	notification.Notify(ctx, s.notifier, notification.Message{
		Event:      notification.EventMTSDecided,
		Subject:    "MTS demand decision recorded",
		Recipients: []string{"PPC"},
		EntityID:   entity.ID(),
	})
	return entity, nil
}

// ListQuery carries inputs for listing demands.
type ListQuery struct {
	Page            int
	PageSize        int
	Search          string
	Type            string
	Status          string
	Month           string
	CpmProductSysID *int64
	// WithoutPlan hides demands that already have a plan item (opt-in, used by
	// the plan-item dialog's demand picker).
	WithoutPlan bool
	SortBy      string
	SortOrder   string
}

// ListResult holds a page of demands plus pagination metadata.
type ListResult struct {
	Items       []*demanddomain.Demand
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of demands (3-tab filters map to
// Type/Status query fields at the delivery boundary).
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := demanddomain.ListFilter{
		Search:          q.Search,
		Type:            q.Type,
		Status:          q.Status,
		Month:           q.Month,
		CpmProductSysID: q.CpmProductSysID,
		WithoutPlan:     q.WithoutPlan,
		Page:            q.Page,
		PageSize:        q.PageSize,
		SortBy:          q.SortBy,
		SortOrder:       q.SortOrder,
	}
	filter.Validate()

	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages(total, filter.PageSize),
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}

func totalPages(total int64, pageSize int) int32 {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	return safeconv.Int64ToInt32((total + int64(pageSize) - 1) / int64(pageSize))
}
