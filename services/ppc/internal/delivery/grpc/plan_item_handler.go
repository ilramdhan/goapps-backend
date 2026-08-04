// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// planItemHandler implements the Layer-2 plan-item RPCs of PPCService.
type planItemHandler struct {
	svc *planitemapp.Service
}

func newPlanItemHandler(svc *planitemapp.Service) *planItemHandler {
	return &planItemHandler{svc: svc}
}

// CreatePlanItem creates a plan item (and cascade INTERMEDIATE child for FG).
func (h *planItemHandler) CreatePlanItem(ctx context.Context, req *ppcv1.CreatePlanItemRequest) (*ppcv1.CreatePlanItemResponse, error) {
	qty, base := decimalField("qty_target", req.GetQtyTarget())
	if base != nil {
		return &ppcv1.CreatePlanItemResponse{Base: base}, nil
	}
	deadline, base := dateField("deadline", req.GetDeadline())
	if base != nil {
		return &ppcv1.CreatePlanItemResponse{Base: base}, nil
	}
	timeline, base := timelineParams(req.PlannedStartDate, req.PlannedDurationDays)
	if base != nil {
		return &ppcv1.CreatePlanItemResponse{Base: base}, nil
	}

	result, err := h.svc.Create(ctx, planitemapp.CreateCommand{
		CpmProductSysID:    req.GetCpmProductSysId(),
		Type:               planItemTypeToString(req.GetType()),
		DemandID:           req.DemandId,
		ParentItemID:       req.ParentItemId,
		QtyTarget:          qty,
		Deadline:           deadline,
		RMSource:           rmSourceToString(req.GetRmSource()),
		MachineGroupID:     req.GetMachineGroupId(),
		PreferredMachineID: req.PreferredMachineId,
		Month:              req.GetMonth(),
		MonthOverride:      req.GetMonthOverride(),
		Timeline:           timeline,
		Notes:              req.GetNotes(),
		CreatedBy:          actorID(ctx),
	})
	if err != nil {
		return &ppcv1.CreatePlanItemResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return createPlanItemResponse(ctx, h.svc, result), nil
}

// createPlanItemResponse shapes a create outcome into its proto response: the
// FG plus every cascade-generated upstream item, with one batched product
// lookup covering the whole chain.
func createPlanItemResponse(ctx context.Context, svc *planitemapp.Service, result *planitemapp.CreateResult) *ppcv1.CreatePlanItemResponse {
	sysIDs := make([]int64, 0, len(result.Children)+1)
	sysIDs = append(sysIDs, result.Item.CpmProductSysID())
	for _, c := range result.Children {
		sysIDs = append(sysIDs, c.CpmProductSysID())
	}
	products := svc.LookupProducts(ctx, sysIDs)

	// The warning rides in its own field, not appended to base.message: a save
	// that yields one item where the planner expected several must explain
	// itself without the client having to split prose apart.
	resp := &ppcv1.CreatePlanItemResponse{
		Base:           successResponse("Plan item created successfully"),
		Data:           planItemToProto(result.Item, products),
		CascadeWarning: result.Warning,
	}
	for _, c := range result.Children {
		resp.Children = append(resp.Children, planItemToProto(c, products))
	}
	// Deprecated single-child field, kept populated for one release so clients
	// built against the pre-cascade response keep working during a rolling deploy.
	if len(resp.Children) > 0 {
		resp.ChildData = resp.Children[0] //nolint:staticcheck // intentional: deprecated field kept populated for one release
	}
	return resp
}

// GetPlanItem retrieves a plan item by ID.
func (h *planItemHandler) GetPlanItem(ctx context.Context, req *ppcv1.GetPlanItemRequest) (*ppcv1.GetPlanItemResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetPlanItemId())
	if err != nil {
		return &ppcv1.GetPlanItemResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	products := h.svc.LookupProducts(ctx, []int64{entity.CpmProductSysID()})
	return &ppcv1.GetPlanItemResponse{Base: successResponse("Plan item retrieved successfully"), Data: planItemToProto(entity, products)}, nil
}

// UpdatePlanItem updates a plan item and logs field changes.
func (h *planItemHandler) UpdatePlanItem(ctx context.Context, req *ppcv1.UpdatePlanItemRequest) (*ppcv1.UpdatePlanItemResponse, error) {
	cmd := planitemapp.UpdateCommand{
		ID:                 req.GetPlanItemId(),
		Sequence:           req.Sequence,
		MachineGroupID:     req.MachineGroupId,
		PreferredMachineID: req.PreferredMachineId,
		Notes:              req.Notes,
		ChangeReason:       req.GetChangeReason(),
		ChangedBy:          actorID(ctx),
	}
	if req.QtyTarget != nil {
		v, base := decimalField("qty_target", req.GetQtyTarget())
		if base != nil {
			return &ppcv1.UpdatePlanItemResponse{Base: base}, nil
		}
		cmd.QtyTarget = &v
	}
	if req.Deadline != nil {
		v, base := dateField("deadline", req.GetDeadline())
		if base != nil {
			return &ppcv1.UpdatePlanItemResponse{Base: base}, nil
		}
		cmd.Deadline = &v
	}
	if req.RmSource != nil {
		s := rmSourceToString(req.GetRmSource())
		cmd.RMSource = &s
	}
	if req.Status != nil {
		s := planItemStatusToString(req.GetStatus())
		cmd.Status = &s
	}
	timeline, base := timelineParams(req.PlannedStartDate, req.PlannedDurationDays)
	if base != nil {
		return &ppcv1.UpdatePlanItemResponse{Base: base}, nil
	}
	cmd.Timeline = timeline

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdatePlanItemResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	products := h.svc.LookupProducts(ctx, []int64{entity.CpmProductSysID()})
	return &ppcv1.UpdatePlanItemResponse{Base: successResponse("Plan item updated successfully"), Data: planItemToProto(entity, products)}, nil
}

// DeletePlanItem deletes a plan item.
func (h *planItemHandler) DeletePlanItem(ctx context.Context, req *ppcv1.DeletePlanItemRequest) (*ppcv1.DeletePlanItemResponse, error) {
	if err := h.svc.Delete(ctx, req.GetPlanItemId()); err != nil {
		return &ppcv1.DeletePlanItemResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeletePlanItemResponse{Base: successResponse("Plan item deleted successfully")}, nil
}

// ListPlanItems lists plan items with filtering and pagination.
func (h *planItemHandler) ListPlanItems(ctx context.Context, req *ppcv1.ListPlanItemsRequest) (*ppcv1.ListPlanItemsResponse, error) {
	result, err := h.svc.List(ctx, planitemapp.ListQuery{
		Page:           int(req.GetPage()),
		PageSize:       int(req.GetPageSize()),
		Search:         req.GetSearch(),
		Month:          req.GetMonth(),
		Type:           planItemTypeToString(req.GetType()),
		Status:         planItemStatusToString(req.GetStatus()),
		MachineGroupID: req.MachineGroupId,
		DemandID:       req.DemandId,
		SortBy:         req.GetSortBy(),
		SortOrder:      req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListPlanItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	products := h.svc.LookupProducts(ctx, planItemSysIDs(result.Items))
	data := make([]*ppcv1.PlanItem, len(result.Items))
	for i, e := range result.Items {
		data[i] = planItemToProto(e, products)
	}
	return &ppcv1.ListPlanItemsResponse{
		Base:       successResponse("Plan items retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// GetGanttView returns plan bars for the timeline (data-only, grouped by group).
func (h *planItemHandler) GetGanttView(ctx context.Context, req *ppcv1.GetGanttViewRequest) (*ppcv1.GetGanttViewResponse, error) {
	from, base := optionalDateField("from_date", req.GetFromDate())
	if base != nil {
		return &ppcv1.GetGanttViewResponse{Base: base}, nil
	}
	to, base := optionalDateField("to_date", req.GetToDate())
	if base != nil {
		return &ppcv1.GetGanttViewResponse{Base: base}, nil
	}

	items, err := h.svc.GetGanttView(ctx, planitemapp.GanttQuery{
		Month:          req.GetMonth(),
		Area:           areaCodeToString(req.GetArea()),
		MachineGroupID: req.MachineGroupId,
		FromDate:       from,
		ToDate:         to,
	})
	if err != nil {
		return &ppcv1.GetGanttViewResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	products := h.svc.LookupProducts(ctx, ganttRowSysIDs(items))
	bars := make([]*ppcv1.GanttBar, len(items))
	for i, row := range items {
		bars[i] = planItemToGanttBar(row, products)
	}
	return &ppcv1.GetGanttViewResponse{Base: successResponse("Gantt view retrieved successfully"), Data: bars}, nil
}

// planItemSysIDs collects the distinct product sys ids referenced by a set of
// plan items, for a single batched product lookup.
func planItemSysIDs(items []*planitemdomain.PlanItem) []int64 {
	ids := make([]int64, len(items))
	for i, e := range items {
		ids[i] = e.CpmProductSysID()
	}
	return ids
}

// ganttRowSysIDs collects the distinct product sys ids referenced by a set of
// Gantt rows, for a single batched product lookup.
func ganttRowSysIDs(rows []*planitemdomain.GanttRow) []int64 {
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.Item.CpmProductSysID()
	}
	return ids
}

// timelineParams decodes the optional planner-supplied timeline fields. Leaving
// both unset yields a zero TimelineParams, which keeps the item system-derived.
func timelineParams(startDate *string, durationDays *int32) (planitemdomain.TimelineParams, *commonv1.BaseResponse) {
	var params planitemdomain.TimelineParams
	if startDate != nil {
		v, base := dateField("planned_start_date", *startDate)
		if base != nil {
			return params, base
		}
		params.StartDate = &v
	}
	if durationDays != nil {
		d := *durationDays
		params.DurationDays = &d
	}
	return params, nil
}

func planItemToProto(e *planitemdomain.PlanItem, products map[int64]*financev1.CostMasterProduct) *ppcv1.PlanItem {
	proto := &ppcv1.PlanItem{
		PlanItemId:      e.ID(),
		CpmProductSysId: e.CpmProductSysID(),
		Type:            stringToPlanItemType(e.Type()),
		QtyTarget:       formatDecimal(e.QtyTarget()),
		Deadline:        formatDate(e.Deadline()),
		RmSource:        stringToRMSource(e.RMSource()),
		Sequence:        e.Sequence(),
		Status:          stringToPlanItemStatus(e.Status()),
		MachineGroupId:  e.MachineGroupID(),
		Month:           e.Month(),
		Notes:           e.Notes(),
		DurationSource:  e.DurationSource(),
		ShadeCode:       e.ShadeCode(),
		ShadeName:       e.ShadeName(),
		Audit:           &commonv1.AuditInfo{CreatedBy: formatInt64(e.CreatedBy()), CreatedAt: e.CreatedAt().Format(time.RFC3339)},
	}
	if e.PlannedStartDate() != nil {
		proto.PlannedStartDate = formatDate(*e.PlannedStartDate())
	}
	if e.PlannedDurationDays() != nil {
		proto.PlannedDurationDays = *e.PlannedDurationDays()
	}
	if e.DemandID() != nil {
		proto.DemandId = *e.DemandID()
	}
	if e.ParentItemID() != nil {
		proto.ParentItemId = *e.ParentItemID()
	}
	if e.PreferredMachineID() != nil {
		proto.PreferredMachineId = *e.PreferredMachineID()
	}
	if e.CarryFromItemID() != nil {
		proto.CarryFromItemId = *e.CarryFromItemID()
	}
	proto.CarryAction = stringToPlanCarryAction(e.CarryAction())
	if p, ok := products[e.CpmProductSysID()]; ok {
		proto.ProductCode = p.GetProductCode()
		proto.ProductName = p.GetProductName()
	}
	return proto
}

// planItemToGanttBar shapes a plan item into a timeline bar. The bar spans the
// stored planned start date through the deadline; when no start date is stored
// yet the bar collapses onto the deadline day rather than inventing a window.
func planItemToGanttBar(row *planitemdomain.GanttRow, products map[int64]*financev1.CostMasterProduct) *ppcv1.GanttBar {
	e := row.Item
	end := e.Deadline()
	start := end
	if s := e.PlannedStartDate(); s != nil {
		start = *s
	}
	bar := &ppcv1.GanttBar{
		PlanItemId:     e.ID(),
		MachineGroupId: e.MachineGroupID(),
		Type:           stringToPlanItemType(e.Type()),
		QtyTarget:      formatDecimal(e.QtyTarget()),
		StartDate:      formatDate(start),
		EndDate:        formatDate(end),
		Status:         stringToPlanItemStatus(e.Status()),
		Area:           stringToAreaCode(row.Area),
		MachineNo:      row.MachineNo,
		WoId:           row.WoID,
		IsChangeover:   row.IsChangeover,
		LotNo:          row.LotNo,
	}
	if e.PreferredMachineID() != nil {
		bar.MachineId = *e.PreferredMachineID()
	}
	if p, ok := products[e.CpmProductSysID()]; ok {
		bar.ProductCode = p.GetProductCode()
		bar.ProductName = p.GetProductName()
	}
	return bar
}
