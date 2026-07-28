// Package workorder provides Layer-3 work-order application usecases (v1.2).
package workorder

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/notification"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// MachineLookup resolves a machine's area for area-match validation.
type MachineLookup interface {
	// MachineArea returns the area code (TXT/SPG/TWT) of a machine, or ("", nil)
	// when the machine does not exist.
	MachineArea(ctx context.Context, machineID int64) (string, error)
}

// LotLookup asserts a lot exists in lot_master.
type LotLookup interface {
	// LotExists reports whether a lot number exists in lot_master.
	LotExists(ctx context.Context, lotNo string) (bool, error)
}

// PlanItemLookup resolves the finance product sys id of a plan item, used to
// drive parameter resolution and snapshots.
type PlanItemLookup interface {
	// ProductSysID returns the cpm_product_sys_id of a plan item.
	ProductSysID(ctx context.Context, planItemID int64) (int64, error)
}

// LotSpecSource resolves the item and shade codes a generated lot is keyed by
// (PRD §9: lot key = item_code + shade_code). Implemented over the finance
// client, which is the only route to finance masters — the databases are
// separate. It is optional; without it a lot cannot be generated and the
// planner must enter one registered in lot_master.
type LotSpecSource interface {
	// LotSpec returns the ERP item code and shade code of a finance product.
	LotSpec(ctx context.Context, productSysID int64) (itemCode, shadeCode string, err error)
}

// LotProvisioner mints a lot number, registers it in lot_master and persists
// the work order in one transaction. Optional: nil disables auto-generation.
type LotProvisioner = workorderdomain.LotProvisioner

// RouteRmSource resolves the RM components of a product's released route. It is
// optional (nil disables route population) and best-effort — finance-degraded
// yields no components rather than an error. Defined in the domain layer so the
// finance client can adapt it directly.
type RouteRmSource = workorderdomain.RouteRmSource

// SnapshotBuilder builds the spec + packing snapshots taken at PM approval. It is
// optional (nil disables snapshotting) and best-effort — a build error leaves the
// snapshot unset rather than blocking approval.
type SnapshotBuilder interface {
	// BuildSnapshots returns the spec and packing snapshot maps for a product.
	BuildSnapshots(ctx context.Context, productSysID int64) (spec, packing map[string]any, err error)
}

// Service bundles WO usecases over the WO repository plus validation/resolution
// ports. All ports except the repository are nil-safe.
type Service struct {
	repo      workorderdomain.Repository
	machines  MachineLookup
	lots      LotLookup
	planItems PlanItemLookup
	resolver  *workorderdomain.Resolver
	snapshots SnapshotBuilder
	routeRms  RouteRmSource
	merge     MergeCandidateSource
	notifier  notification.Notifier
	lotSpecs  LotSpecSource
	lotProv   LotProvisioner
}

// Deps carries the (nil-safe) collaborators for the WO service.
type Deps struct {
	Machines  MachineLookup
	Lots      LotLookup
	PlanItems PlanItemLookup
	Resolver  *workorderdomain.Resolver
	Snapshots SnapshotBuilder
	RouteRms  RouteRmSource
	Merge     MergeCandidateSource
	Notifier  notification.Notifier
	LotSpecs  LotSpecSource
	LotProv   LotProvisioner
}

// NewService builds a WO application service. All deps except the repository are
// nil-safe (nil disables the corresponding validation/resolution/notify).
func NewService(repo workorderdomain.Repository, deps Deps) *Service {
	return &Service{
		repo:      repo,
		machines:  deps.Machines,
		lots:      deps.Lots,
		planItems: deps.PlanItems,
		resolver:  deps.Resolver,
		snapshots: deps.Snapshots,
		routeRms:  deps.RouteRms,
		merge:     deps.Merge,
		notifier:  deps.Notifier,
		lotSpecs:  deps.LotSpecs,
		lotProv:   deps.LotProv,
	}
}

// CreateCommand carries inputs for creating a WO.
type CreateCommand struct {
	AreaCode            string
	PlanItemID          int64
	MachineID           int64
	CrhHeadID           int64
	CrhVersion          int32
	LotNo               string
	DemandID            *int64
	QtyTarget           float64
	GradeRequirement    string
	Deadline            time.Time
	ProdCategory        string
	AutoApproveDisabled bool
	CreatedBy           int64
	// Merge (explicit planner action, never automatic): extra plan items this
	// WO also covers. QtyContributions is positionally aligned; a zero or
	// missing entry defaults to that plan item's own target.
	AdditionalPlanItemIDs []int64
	QtyContributions      []float64
}

// Create validates the lot + machine-area, persists a DRAFT WO, then materializes
// its planned parameter rows (display_group=Machine) as PPC values via the
// resolution chain.
//
// Both lot paths end up validated. A supplied lot must already exist in
// lot_master and is rejected otherwise — never silently created. A blank lot is
// generated, registered in lot_master and persisted with the work order in one
// transaction, so an auto-generated lot is a real lot rather than a string the
// ETL cannot price into kilograms.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*workorderdomain.WorkOrder, error) {
	if err := s.validateMachineArea(ctx, cmd.MachineID, cmd.AreaCode); err != nil {
		return nil, err
	}
	links, qtyTarget, err := s.buildPlanItemLinks(ctx, cmd)
	if err != nil {
		return nil, err
	}
	build := func(lotNo string) (*workorderdomain.WorkOrder, error) {
		return workorderdomain.New(workorderdomain.NewParams{
			WoNo:                generateWoNo(cmd.AreaCode),
			LotNo:               lotNo,
			AreaCode:            cmd.AreaCode,
			MachineID:           cmd.MachineID,
			CrhHeadID:           cmd.CrhHeadID,
			CrhVersion:          cmd.CrhVersion,
			PlanItemID:          cmd.PlanItemID,
			DemandID:            cmd.DemandID,
			QtyTarget:           qtyTarget,
			GradeRequirement:    cmd.GradeRequirement,
			Deadline:            cmd.Deadline,
			ProdCategory:        cmd.ProdCategory,
			AutoApproveDisabled: cmd.AutoApproveDisabled,
			CreatedBy:           cmd.CreatedBy,
			PlanItemLinks:       links,
		})
	}

	entity, err := s.createWithLot(ctx, cmd, build)
	if err != nil {
		return nil, err
	}
	if err := s.materializeParameters(ctx, entity, nil); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, entity.ID())
}

// createWithLot resolves the WO's lot and persists the WO. A supplied lot is
// validated against lot_master first; a blank one is generated and registered
// through the provisioner, sharing the WO's transaction.
func (s *Service) createWithLot(
	ctx context.Context, cmd CreateCommand, build func(lotNo string) (*workorderdomain.WorkOrder, error),
) (*workorderdomain.WorkOrder, error) {
	if cmd.LotNo != "" {
		if err := s.validateLot(ctx, cmd.LotNo); err != nil {
			return nil, err
		}
		entity, err := build(cmd.LotNo)
		if err != nil {
			return nil, err
		}
		if err := s.repo.Create(ctx, entity); err != nil {
			return nil, err
		}
		return entity, nil
	}

	if s.lotProv == nil {
		return nil, workorderdomain.ErrLotGenerationUnavailable
	}
	req, err := s.lotProvisionRequest(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return s.lotProv.CreateWithGeneratedLot(ctx, req, build)
}

// lotProvisionRequest assembles the lot_master row a generated lot needs: the
// item + shade codes it is keyed by, and its standard bobbin weights.
//
// Every input has to come from a real source. The item/shade codes come from
// the plan item's finance product; the full standard weight from the resolved
// STD_WEIGHT parameter. When any of them is missing the WO is rejected rather
// than registered against invented weights, which would silently mis-price
// every bobbin the ETL later attributes to it.
func (s *Service) lotProvisionRequest(ctx context.Context, cmd CreateCommand) (workorderdomain.LotProvisionRequest, error) {
	req := workorderdomain.LotProvisionRequest{
		AreaCode:  cmd.AreaCode,
		Year:      time.Now().Year(),
		Notes:     workorderdomain.GeneratedLotNote,
		CreatedBy: strconv.FormatInt(cmd.CreatedBy, 10),
	}
	if s.planItems == nil || s.lotSpecs == nil {
		return req, workorderdomain.ErrLotSpecUnavailable
	}
	productSysID, err := s.planItems.ProductSysID(ctx, cmd.PlanItemID)
	if err != nil {
		return req, err
	}
	itemCode, shadeCode, err := s.lotSpecs.LotSpec(ctx, productSysID)
	if err != nil {
		return req, err
	}
	req.ItemCode = itemCode
	req.ShadeCode = shadeCode

	stdFull, err := s.resolveStdWeight(ctx, productSysID, cmd.MachineID)
	if err != nil {
		return req, err
	}
	req.StdWeightFull = stdFull
	req.StdWeightUnfull = stdFull * workorderdomain.LotUnfullWeightRatio

	if err := req.Validate(); err != nil {
		return req, err
	}
	return req, nil
}

// resolveStdWeight reads the STD_WEIGHT well-known parameter for a product on a
// machine. Zero (or an unavailable resolver) means the caller cannot register a
// lot.
func (s *Service) resolveStdWeight(ctx context.Context, productSysID, machineID int64) (float64, error) {
	if s.resolver == nil {
		return 0, workorderdomain.ErrLotSpecUnavailable
	}
	resolved, err := s.resolver.Resolve(ctx, workorderdomain.ResolveRequest{
		ProductSysID: productSysID,
		MachineID:    machineID,
	})
	if err != nil {
		return 0, err
	}
	for _, rp := range resolved {
		if rp.ParamCode == workorderdomain.WellKnownStdWeight && rp.Num != nil && *rp.Num > 0 {
			return *rp.Num, nil
		}
	}
	return 0, workorderdomain.ErrLotSpecUnavailable
}

// materializeParameters resolves the Machine-group params for the WO's product +
// machine and persists them as PPC values (PC defaults to PPC). Skipped when the
// resolver or plan-item lookup is unavailable.
func (s *Service) materializeParameters(ctx context.Context, entity *workorderdomain.WorkOrder, refWoID *int64) error {
	if s.resolver == nil || s.planItems == nil {
		return nil
	}
	productSysID, err := s.planItems.ProductSysID(ctx, entity.PlanItemID())
	if err != nil {
		return err
	}
	resolved, err := s.resolver.Resolve(ctx, workorderdomain.ResolveRequest{
		ProductSysID: productSysID,
		MachineID:    entity.MachineID(),
		RefWoID:      refWoID,
		DisplayGroup: displayGroupMachine,
	})
	if err != nil {
		return err
	}
	params := make([]*workorderdomain.Parameter, 0, len(resolved))
	for _, rp := range resolved {
		params = append(params, resolvedToPPCParameter(entity.ID(), rp))
	}
	if err := s.repo.ReplaceParameters(ctx, entity.ID(), params); err != nil {
		return err
	}
	return nil
}

// validateLot asserts a lot exists in lot_master. A lot the planner typed is
// never created implicitly: an unknown lot is a typo far more often than it is
// a new lot, and a silently-created one has no standard weights, so the ETL
// would compute zero kilograms for every bobbin booked against it.
func (s *Service) validateLot(ctx context.Context, lotNo string) error {
	if s.lots == nil {
		return nil
	}
	ok, err := s.lots.LotExists(ctx, lotNo)
	if err != nil {
		return err
	}
	if !ok {
		return workorderdomain.ErrLotNotFound
	}
	return nil
}

func (s *Service) validateMachineArea(ctx context.Context, machineID int64, areaCode string) error {
	if s.machines == nil {
		return nil
	}
	machineArea, err := s.machines.MachineArea(ctx, machineID)
	if err != nil {
		return err
	}
	if machineArea == "" {
		return workorderdomain.ErrInvalidMachine
	}
	if machineArea != areaCode {
		return workorderdomain.ErrMachineAreaMismatch
	}
	return nil
}

// Get retrieves a WO by ID (with parameters + RM allocations + production actuals).
func (s *Service) Get(ctx context.Context, id int64) (*workorderdomain.WorkOrder, []*workorderdomain.ProductionActual, error) {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	actuals, err := s.repo.GetProductionActuals(ctx, id, nil, "")
	if err != nil {
		return nil, nil, err
	}
	return entity, actuals, nil
}

// UpdateCommand carries editable WO header fields.
type UpdateCommand struct {
	ID                  int64
	MachineID           *int64
	LotNo               *string
	QtyTarget           *float64
	Deadline            *time.Time
	GradeRequirement    *string
	ProdCategory        *string
	AutoApproveDisabled *bool
	RevisionReason      *string
}

// Update mutates a WO header.
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*workorderdomain.WorkOrder, error) {
	entity, err := s.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := entity.Update(workorderdomain.UpdateParams{
		MachineID:           cmd.MachineID,
		LotNo:               cmd.LotNo,
		QtyTarget:           cmd.QtyTarget,
		Deadline:            cmd.Deadline,
		GradeRequirement:    cmd.GradeRequirement,
		ProdCategory:        cmd.ProdCategory,
		AutoApproveDisabled: cmd.AutoApproveDisabled,
		RevisionReason:      cmd.RevisionReason,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, entity.ID())
}

// Delete removes a WO by ID. Only DRAFT work orders may be deleted.
func (s *Service) Delete(ctx context.Context, id int64) error {
	entity, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if entity.Status() != workorderdomain.StatusDraft {
		return workorderdomain.ErrNotDeletable
	}
	return s.repo.Delete(ctx, id)
}

// ListQuery carries inputs for listing WOs.
type ListQuery struct {
	Page       int
	PageSize   int
	Search     string
	Area       string
	Status     string
	MachineID  *int64
	PlanItemID *int64
	LotNo      string
	SortBy     string
	SortOrder  string
}

// ListResult holds a page of WOs plus pagination metadata.
type ListResult struct {
	Items       []*workorderdomain.WorkOrder
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// List retrieves a filtered, paginated page of WOs.
func (s *Service) List(ctx context.Context, q ListQuery) (*ListResult, error) {
	filter := workorderdomain.ListFilter{
		Search:     q.Search,
		Area:       q.Area,
		Status:     q.Status,
		MachineID:  q.MachineID,
		PlanItemID: q.PlanItemID,
		LotNo:      q.LotNo,
		Page:       q.Page,
		PageSize:   q.PageSize,
		SortBy:     q.SortBy,
		SortOrder:  q.SortOrder,
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

// ResolveQuery carries inputs for a parameter-resolution preview.
type ResolveQuery struct {
	ProductSysID int64
	MachineID    int64
	RefWoID      *int64
	DisplayGroup string
}

// ResolveParameters returns a preview of resolved parameter values without
// persisting. Returns an empty slice when the resolver is unavailable.
func (s *Service) ResolveParameters(ctx context.Context, q ResolveQuery) ([]workorderdomain.ResolvedParam, error) {
	if s.resolver == nil {
		return nil, nil
	}
	return s.resolver.Resolve(ctx, workorderdomain.ResolveRequest{
		ProductSysID: q.ProductSysID,
		MachineID:    q.MachineID,
		RefWoID:      q.RefWoID,
		DisplayGroup: q.DisplayGroup,
	})
}

// displayGroupMachine is the display group whose params flow into WO_PARAMETER.
const displayGroupMachine = "Machine"

// generateWoNo builds a unique WO number from the area code and a timestamp
// (Phase-1; a per-area sequence is a later refinement).
func generateWoNo(areaCode string) string {
	return fmt.Sprintf("WO-%s-%d", areaCode, time.Now().UnixNano())
}
