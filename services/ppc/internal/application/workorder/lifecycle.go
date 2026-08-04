package workorder

import (
	"context"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/notification"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ParamValueInput is a typed PPC/PC parameter value keyed by param id.
type ParamValueInput struct {
	ParamID string
	Num     *float64
	Text    *string
	Flag    *bool
	HasFlag bool
}

// SaveWOParametersCommand carries PPC parameter edits.
type SaveWOParametersCommand struct {
	WOID   int64
	Values []ParamValueInput
}

// SaveWOParameters applies PPC value edits to a WO's planned parameters (DRAFT).
func (s *Service) SaveWOParameters(ctx context.Context, cmd SaveWOParametersCommand) (*workorderdomain.WorkOrder, error) {
	existing, err := s.repo.ListParameters(ctx, cmd.WOID)
	if err != nil {
		return nil, err
	}
	byID := indexParams(existing)
	for _, in := range cmd.Values {
		p, ok := byID[in.ParamID]
		if !ok {
			continue
		}
		applyPPCValue(p, in)
		if err := s.repo.SetParameterPPCValue(ctx, cmd.WOID, p); err != nil {
			return nil, err
		}
	}
	return s.repo.GetByID(ctx, cmd.WOID)
}

// Submit moves a DRAFT WO to SUBMITTED and notifies PC. Optional PPC values are
// applied before the transition.
func (s *Service) Submit(ctx context.Context, woID int64, values []ParamValueInput) (*workorderdomain.WorkOrder, error) {
	if len(values) > 0 {
		if _, err := s.SaveWOParameters(ctx, SaveWOParametersCommand{WOID: woID, Values: values}); err != nil {
			return nil, err
		}
	}
	entity, err := s.repo.GetByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	if err := entity.Submit(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	notification.Notify(ctx, s.notifier, notification.Message{
		Event:      notification.EventWOSubmitted,
		Subject:    "Work order submitted for approval",
		Recipients: []string{"PC"},
		EntityID:   entity.ID(),
	})
	return entity, nil
}

// ApproveWOParameter records the PC approval: fills PC values (defaulting to PPC
// where omitted) and moves the WO to PC_APPROVED.
func (s *Service) ApproveWOParameter(ctx context.Context, woID int64, pcValues []ParamValueInput, userID int64) (*workorderdomain.WorkOrder, error) {
	entity, err := s.repo.GetByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	if _, err := entity.ApprovePC(userID, time.Now()); err != nil {
		return nil, err
	}
	if err := s.fillPCValues(ctx, woID, pcValues); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	notification.Notify(ctx, s.notifier, notification.Message{
		Event:      notification.EventWOPCApproved,
		Subject:    "Work order parameters approved by PC",
		Recipients: []string{"PM"},
		EntityID:   entity.ID(),
	})
	return s.repo.GetByID(ctx, woID)
}

// fillPCValues persists the PC value of each parameter: an explicit input value
// when provided, otherwise the PPC value (default = PPC).
func (s *Service) fillPCValues(ctx context.Context, woID int64, pcValues []ParamValueInput) error {
	params, err := s.repo.ListParameters(ctx, woID)
	if err != nil {
		return err
	}
	overrides := indexInputs(pcValues)
	for _, p := range params {
		if in, ok := overrides[p.ParamID]; ok {
			applyPCValue(p, in)
		} else {
			mirrorPPCIntoPC(p)
		}
		if err := s.repo.SetParameterPCValue(ctx, woID, p); err != nil {
			return err
		}
	}
	return nil
}

// ApproveWO records an approval side (PC/PM). PM approval snapshots spec/packing.
func (s *Service) ApproveWO(ctx context.Context, woID int64, side string, userID int64) (*workorderdomain.WorkOrder, error) {
	if side == workorderdomain.ApprovalSidePC {
		return s.ApproveWOParameter(ctx, woID, nil, userID)
	}
	return s.approvePM(ctx, woID, userID)
}

func (s *Service) approvePM(ctx context.Context, woID, userID int64) (*workorderdomain.WorkOrder, error) {
	entity, err := s.repo.GetByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	fullyApproved, err := entity.ApprovePM(userID, time.Now())
	if err != nil {
		return nil, err
	}
	s.applySnapshots(ctx, entity)
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	if fullyApproved {
		notification.Notify(ctx, s.notifier, notification.Message{
			Event:      notification.EventWOApproved,
			Subject:    "Work order approved",
			Recipients: []string{"PPC"},
			EntityID:   entity.ID(),
		})
	}
	return entity, nil
}

// applySnapshots builds + records spec/packing snapshots at PM approval (best
// effort — a build failure leaves the snapshots unset).
func (s *Service) applySnapshots(ctx context.Context, entity *workorderdomain.WorkOrder) {
	if s.snapshots == nil || s.planItems == nil {
		return
	}
	productSysID, err := s.planItems.ProductSysID(ctx, entity.PlanItemID())
	if err != nil {
		return
	}
	spec, packing, err := s.snapshots.BuildSnapshots(ctx, productSysID)
	if err != nil {
		return
	}
	entity.SetSnapshots(spec, packing)
}

// Reject sends a submitted WO back to PPC with a reason.
func (s *Service) Reject(ctx context.Context, woID int64, reason string) (*workorderdomain.WorkOrder, error) {
	entity, err := s.repo.GetByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	if err := entity.Reject(reason); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	notification.Notify(ctx, s.notifier, notification.Message{
		Event:      notification.EventWORejected,
		Subject:    "Work order rejected",
		Recipients: []string{"PPC"},
		EntityID:   entity.ID(),
	})
	return entity, nil
}

// CreateWOReferenceCommand carries inputs for a duplicate/continuation WO.
type CreateWOReferenceCommand struct {
	SourceWOID int64
	RefType    string // TEMPLATE / CONTINUATION
	LotNo      string
	QtyTarget  float64
	Deadline   time.Time
	MachineID  *int64
	CreatedBy  int64
}

// CreateWOReference creates a new DRAFT WO from a source WO. TEMPLATE copies the
// PPC params as a starting point with no binding; CONTINUATION hard-links the
// source (ref_wo_id) and inherits its demand, seeding params from the reference.
func (s *Service) CreateWOReference(ctx context.Context, cmd CreateWOReferenceCommand) (*workorderdomain.WorkOrder, error) {
	if !workorderdomain.IsValidRefType(cmd.RefType) {
		return nil, workorderdomain.ErrInvalidRefType
	}
	src, err := s.repo.GetByID(ctx, cmd.SourceWOID)
	if err != nil {
		return nil, err
	}
	machineID := src.MachineID()
	if cmd.MachineID != nil {
		machineID = *cmd.MachineID
	}
	if err := s.validateMachineArea(ctx, machineID, src.AreaCode()); err != nil {
		return nil, err
	}
	refWoID := continuationRef(cmd.RefType, src)
	// Seeded before the write so the parameters share the new WO's transaction: a
	// carry-forward that committed its header and lost its parameters would look
	// like a valid WO the PC operator simply cannot run.
	params, err := s.referenceParameters(ctx, src, machineID, refWoID)
	if err != nil {
		return nil, err
	}
	build := func(lotNo string) (*workorderdomain.WorkOrder, error) {
		entity, err := s.buildReference(cmd, src, machineID, lotNo, refWoID)
		if err != nil {
			return nil, err
		}
		entity.AttachParameters(params)
		return entity, nil
	}

	entity, err := s.createReferenceWithLot(ctx, cmd, src, machineID, build)
	if err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, entity.ID())
}

// createReferenceWithLot resolves the reference WO's lot the same way an
// ordinary create does. A carry-forward never asks the planner for a lot — the
// continuation is a fresh production run and needs its own — so a blank lot is
// minted and registered in lot_master rather than rejected. A lot the planner
// did type still has to exist, so this path cannot reopen the hole where a WO
// points at a lot with no standard weights.
func (s *Service) createReferenceWithLot(
	ctx context.Context,
	cmd CreateWOReferenceCommand,
	src *workorderdomain.WorkOrder,
	machineID int64,
	build func(lotNo string) (*workorderdomain.WorkOrder, error),
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
	req, err := s.lotProvisionRequest(ctx, lotInputs{
		AreaCode: src.AreaCode(), PlanItemID: src.PlanItemID(),
		MachineID: machineID, CreatedBy: cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	return s.lotProv.CreateWithGeneratedLot(ctx, req, build)
}

// continuationRef returns the source WO id a CONTINUATION hard-links, or nil
// for a TEMPLATE duplicate, which deliberately carries no binding.
func continuationRef(refType string, src *workorderdomain.WorkOrder) *int64 {
	if refType != workorderdomain.RefTypeContinuation {
		return nil
	}
	id := src.ID()
	return &id
}

func (s *Service) buildReference(
	cmd CreateWOReferenceCommand, src *workorderdomain.WorkOrder, machineID int64, lotNo string, refWoID *int64,
) (*workorderdomain.WorkOrder, error) {
	var demandID *int64
	if cmd.RefType == workorderdomain.RefTypeContinuation {
		demandID = src.DemandID()
	}
	entity, err := workorderdomain.New(workorderdomain.NewParams{
		WoNo:             generateWoNo(src.AreaCode()),
		LotNo:            lotNo,
		AreaCode:         src.AreaCode(),
		MachineID:        machineID,
		CrhHeadID:        src.CrhHeadID(),
		CrhVersion:       src.CrhVersion(),
		PlanItemID:       src.PlanItemID(),
		DemandID:         demandID,
		RefWoID:          refWoID,
		RefType:          cmd.RefType,
		QtyTarget:        cmd.QtyTarget,
		GradeRequirement: src.GradeRequirement(),
		Deadline:         cmd.Deadline,
		ProdCategory:     src.ProdCategory(),
		CreatedBy:        cmd.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// referenceParameters builds the parameters a reference WO starts from, as PPC
// values (CONTINUATION passes the ref for the resolver's top layer). Falls back
// to a direct copy of the source WO's parameters when the resolver is
// unavailable. Read-only: the caller attaches the result to the new WO so both
// are written together, and the repository stamps the WO id.
func (s *Service) referenceParameters(
	ctx context.Context, src *workorderdomain.WorkOrder, machineID int64, refWoID *int64,
) ([]*workorderdomain.Parameter, error) {
	if s.resolver != nil && s.planItems != nil {
		return s.resolveParameters(ctx, src.PlanItemID(), machineID, refWoID)
	}
	srcParams, err := s.repo.ListParameters(ctx, src.ID())
	if err != nil {
		return nil, err
	}
	copied := make([]*workorderdomain.Parameter, 0, len(srcParams))
	for _, sp := range srcParams {
		copied = append(copied, copyParamAsPPC(0, sp))
	}
	return copied, nil
}

// SaveWORmAllocationsCommand carries RM allocation lines (from cost_route_rm).
type SaveWORmAllocationsCommand struct {
	WOID        int64
	Allocations []RmAllocationInput
}

// RmAllocationInput is one RM allocation line.
type RmAllocationInput struct {
	CrmRmID      int64
	RmType       string
	LotNo        string
	RmSource     string
	FreshBox     string
	ShadeCode    string
	QtyAllocated float64
	Notes        string
}

// SaveWORmAllocations replaces a WO's RM allocation lines.
func (s *Service) SaveWORmAllocations(ctx context.Context, cmd SaveWORmAllocationsCommand) ([]*workorderdomain.RmAllocation, error) {
	if _, err := s.repo.GetByID(ctx, cmd.WOID); err != nil {
		return nil, err
	}
	allocs := make([]*workorderdomain.RmAllocation, 0, len(cmd.Allocations))
	for _, in := range cmd.Allocations {
		allocs = append(allocs, &workorderdomain.RmAllocation{
			WOID:         cmd.WOID,
			CrmRmID:      in.CrmRmID,
			RmType:       in.RmType,
			LotNo:        in.LotNo,
			RmSource:     in.RmSource,
			FreshBox:     in.FreshBox,
			ShadeCode:    in.ShadeCode,
			QtyAllocated: in.QtyAllocated,
			Notes:        in.Notes,
		})
	}
	if err := s.repo.ReplaceRmAllocations(ctx, cmd.WOID, allocs); err != nil {
		return nil, err
	}
	return s.repo.ListRmAllocations(ctx, cmd.WOID)
}

// PopulateRmFromRouteCommand drives RM-BOM auto-population from a WO's route.
type PopulateRmFromRouteCommand struct {
	WOID    int64
	Replace bool // also persist the suggestions (replace stored lines)
}

// PopulateRmFromRoute materializes RM allocation suggestions from the WO's
// product route (cost_route_rm). Each route RM component becomes a line with a
// ratio-derived qty (route_rm_ratio × wo qty_target) and shade from the route
// stage; lot/source are left for PPC to fill. When cmd.Replace is set the
// suggestions also replace the stored allocations. Requires the route source
// and plan-item lookup; without them it returns the current stored lines.
func (s *Service) PopulateRmFromRoute(ctx context.Context, cmd PopulateRmFromRouteCommand) ([]*workorderdomain.RmAllocation, error) {
	wo, err := s.repo.GetByID(ctx, cmd.WOID)
	if err != nil {
		return nil, err
	}
	if s.routeRms == nil || s.planItems == nil {
		return s.repo.ListRmAllocations(ctx, cmd.WOID)
	}
	productSysID, err := s.planItems.ProductSysID(ctx, wo.PlanItemID())
	if err != nil {
		return nil, err
	}
	comps, err := s.routeRms.RouteRmComponents(ctx, productSysID)
	if err != nil {
		return nil, err
	}
	suggestions := make([]*workorderdomain.RmAllocation, 0, len(comps))
	for _, c := range comps {
		suggestions = append(suggestions, &workorderdomain.RmAllocation{
			WOID:         cmd.WOID,
			CrmRmID:      c.CrmRmID,
			RmType:       c.RmType,
			ShadeCode:    c.ShadeCode,
			QtyAllocated: c.Ratio * wo.QtyTarget(),
		})
	}
	if cmd.Replace {
		if err := s.repo.ReplaceRmAllocations(ctx, cmd.WOID, suggestions); err != nil {
			return nil, err
		}
		return s.repo.ListRmAllocations(ctx, cmd.WOID)
	}
	return suggestions, nil
}

// SaveWOExecutionCommand carries one actual parameter value (per date+shift+param).
type SaveWOExecutionCommand struct {
	WOID    int64
	Date    time.Time
	Shift   string
	ParamID string
	Num     *float64
	Text    *string
	Flag    *bool
	InputBy int64
}

// SaveWOExecution upserts one actual parameter value for a WO.
func (s *Service) SaveWOExecution(ctx context.Context, cmd SaveWOExecutionCommand) (*workorderdomain.Execution, error) {
	if _, err := s.repo.GetByID(ctx, cmd.WOID); err != nil {
		return nil, err
	}
	exec := &workorderdomain.Execution{
		WOID:      cmd.WOID,
		Date:      cmd.Date,
		Shift:     cmd.Shift,
		ParamID:   cmd.ParamID,
		ValueNum:  cmd.Num,
		ValueText: cmd.Text,
		ValueFlag: cmd.Flag,
		InputBy:   cmd.InputBy,
		InputAt:   time.Now(),
	}
	if err := s.repo.UpsertExecution(ctx, exec); err != nil {
		return nil, err
	}
	return exec, nil
}

// ListExecutions lists a WO's actual parameter values.
func (s *Service) ListExecutions(ctx context.Context, woID int64) ([]*workorderdomain.Execution, error) {
	return s.repo.ListExecutions(ctx, woID)
}

// ListRmAllocations lists a WO's RM allocations.
func (s *Service) ListRmAllocations(ctx context.Context, woID int64) ([]*workorderdomain.RmAllocation, error) {
	return s.repo.ListRmAllocations(ctx, woID)
}

// GetProductionActuals lists production-actual rows for a WO (optionally scoped).
func (s *Service) GetProductionActuals(ctx context.Context, woID int64, date *time.Time, shift string) ([]*workorderdomain.ProductionActual, error) {
	return s.repo.GetProductionActuals(ctx, woID, date, shift)
}

// AdjustWOActualCommand carries a two-axis qty_actual adjustment.
type AdjustWOActualCommand struct {
	WOID      int64
	Date      time.Time
	Shift     string
	QtyActual float64
	Reason    string
	EditedBy  int64
}

// AdjustWOActual sets qty_actual (source=ADJUSTED) for a (wo,date,shift) row and
// records a wo_actual_log entry. A reason is required.
func (s *Service) AdjustWOActual(ctx context.Context, cmd AdjustWOActualCommand) (*workorderdomain.ProductionActual, error) {
	if cmd.Reason == "" {
		return nil, workorderdomain.ErrEmptyReason
	}
	return s.repo.AdjustActual(ctx, cmd.WOID, cmd.Date, cmd.Shift, cmd.QtyActual, cmd.Reason, cmd.EditedBy)
}
