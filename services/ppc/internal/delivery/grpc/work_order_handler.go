// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// workOrderHandler implements the Layer-3 work-order RPCs of PPCService (v1.2).
// It also holds the plan-item service so merge candidates (which are plan-item
// ids) can be rendered as full plan items without a second round trip from the
// client.
type workOrderHandler struct {
	svc       *workorderapp.Service
	planItems *planitemapp.Service
}

func newWorkOrderHandler(svc *workorderapp.Service, planItems *planitemapp.Service) *workOrderHandler {
	return &workOrderHandler{svc: svc, planItems: planItems}
}

// CreateWorkOrder creates a DRAFT work order and materializes its parameters.
func (h *workOrderHandler) CreateWorkOrder(ctx context.Context, req *ppcv1.CreateWorkOrderRequest) (*ppcv1.CreateWorkOrderResponse, error) {
	qty, base := decimalField("qty_target", req.GetQtyTarget())
	if base != nil {
		return &ppcv1.CreateWorkOrderResponse{Base: base}, nil
	}
	deadline, base := dateField("deadline", req.GetDeadline())
	if base != nil {
		return &ppcv1.CreateWorkOrderResponse{Base: base}, nil
	}
	contributions, base := qtyContributions(req.GetQtyContributions())
	if base != nil {
		return &ppcv1.CreateWorkOrderResponse{Base: base}, nil
	}

	entity, err := h.svc.Create(ctx, workorderapp.CreateCommand{
		AreaCode:            areaCodeToString(req.GetArea()),
		PlanItemID:          req.GetPlanItemId(),
		MachineID:           req.GetMachineId(),
		CrhHeadID:           req.GetCrhHeadId(),
		CrhVersion:          req.GetCrhVersion(),
		LotNo:               req.GetLotNo(),
		DemandID:            req.DemandId,
		QtyTarget:           qty,
		GradeRequirement:    req.GetGradeRequirement(),
		Deadline:            deadline,
		ProdCategory:        prodCategoryToString(req.GetProdCategory()),
		AutoApproveDisabled: req.GetAutoApproveDisabled(),
		CreatedBy:           actorID(ctx),

		AdditionalPlanItemIDs: req.GetAdditionalPlanItemIds(),
		QtyContributions:      contributions,
	})
	if err != nil {
		return &ppcv1.CreateWorkOrderResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateWorkOrderResponse{
		Base: successResponse("Work order created successfully"),
		Data: h.decorateWO(ctx, entity, nil),
	}, nil
}

// decorateWO maps a WO to proto and fills the linked plan items with the
// product/shade/deadline projection the UI needs. Decoration is best-effort:
// an unresolvable plan item leaves those labels empty rather than failing the
// whole response, since the link itself is still valid.
func (h *workOrderHandler) decorateWO(
	ctx context.Context, e *workorderdomain.WorkOrder, actuals []*workorderdomain.ProductionActual,
) *ppcv1.WorkOrder {
	proto := workOrderToProto(e, actuals)
	if h.planItems == nil || len(proto.GetLinkedPlanItems()) == 0 {
		return proto
	}
	items := make(map[int64]*planitemdomain.PlanItem, len(proto.GetLinkedPlanItems()))
	sysIDs := make([]int64, 0, len(proto.GetLinkedPlanItems()))
	for _, l := range proto.GetLinkedPlanItems() {
		item, err := h.planItems.Get(ctx, l.GetPlanItemId())
		if err != nil {
			continue
		}
		items[l.GetPlanItemId()] = item
		sysIDs = append(sysIDs, item.CpmProductSysID())
	}
	products := h.planItems.LookupProducts(ctx, sysIDs)
	for _, l := range proto.GetLinkedPlanItems() {
		item, ok := items[l.GetPlanItemId()]
		if !ok {
			continue
		}
		l.ShadeCode = item.ShadeCode()
		l.ShadeName = item.ShadeName()
		l.Deadline = formatDate(item.Deadline())
		if p, ok := products[item.CpmProductSysID()]; ok {
			l.ProductCode = p.GetProductCode()
			l.ProductName = p.GetProductName()
		}
	}
	return proto
}

// qtyContributions parses the positional decimal-as-string contributions that
// accompany additional_plan_item_ids. An empty entry means "use that plan
// item's own target", so it parses to zero rather than an error.
func qtyContributions(values []string) ([]float64, *commonv1.BaseResponse) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]float64, 0, len(values))
	for _, v := range values {
		if v == "" {
			out = append(out, 0)
			continue
		}
		qty, base := decimalField("qty_contributions", v)
		if base != nil {
			return nil, base
		}
		out = append(out, qty)
	}
	return out, nil
}

// ListMergeCandidates lists the plan items that may join one work order with
// the anchor plan item (same product, machine group, compatible shade, nearby
// deadline, not already covered by another WO).
func (h *workOrderHandler) ListMergeCandidates(ctx context.Context, req *ppcv1.ListMergeCandidatesRequest) (*ppcv1.ListMergeCandidatesResponse, error) {
	ids, err := h.svc.ListMergeCandidates(ctx, req.GetAnchorPlanItemId(), req.GetWindowDays())
	if err != nil {
		return &ppcv1.ListMergeCandidatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data, err := h.loadPlanItems(ctx, ids)
	if err != nil {
		return &ppcv1.ListMergeCandidatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ListMergeCandidatesResponse{
		Base: successResponse("Merge candidates retrieved successfully"),
		Data: data,
	}, nil
}

// loadPlanItems hydrates plan-item ids into proto plan items with one batched
// product lookup covering the whole set (same precedent as createPlanItemResponse).
func (h *workOrderHandler) loadPlanItems(ctx context.Context, ids []int64) ([]*ppcv1.PlanItem, error) {
	if h.planItems == nil || len(ids) == 0 {
		return nil, nil
	}
	items := make([]*planitemdomain.PlanItem, 0, len(ids))
	sysIDs := make([]int64, 0, len(ids))
	for _, id := range ids {
		item, err := h.planItems.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		sysIDs = append(sysIDs, item.CpmProductSysID())
	}
	products := h.planItems.LookupProducts(ctx, sysIDs)
	data := make([]*ppcv1.PlanItem, len(items))
	for i, item := range items {
		data[i] = planItemToProto(item, products)
	}
	return data, nil
}

// GetWorkOrder retrieves a WO with its parameters + production actuals.
func (h *workOrderHandler) GetWorkOrder(ctx context.Context, req *ppcv1.GetWorkOrderRequest) (*ppcv1.GetWorkOrderResponse, error) {
	entity, actuals, err := h.svc.Get(ctx, req.GetWoId())
	if err != nil {
		return &ppcv1.GetWorkOrderResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetWorkOrderResponse{Base: successResponse("Work order retrieved successfully"), Data: h.decorateWO(ctx, entity, actuals)}, nil
}

// UpdateWorkOrder updates a WO header.
func (h *workOrderHandler) UpdateWorkOrder(ctx context.Context, req *ppcv1.UpdateWorkOrderRequest) (*ppcv1.UpdateWorkOrderResponse, error) {
	cmd := workorderapp.UpdateCommand{
		ID:                  req.GetWoId(),
		MachineID:           req.MachineId,
		LotNo:               req.LotNo,
		GradeRequirement:    req.GradeRequirement,
		AutoApproveDisabled: req.AutoApproveDisabled,
		RevisionReason:      req.RevisionReason,
	}
	if req.ProdCategory != nil {
		v := prodCategoryToString(req.GetProdCategory())
		cmd.ProdCategory = &v
	}
	if req.QtyTarget != nil {
		v, base := decimalField("qty_target", req.GetQtyTarget())
		if base != nil {
			return &ppcv1.UpdateWorkOrderResponse{Base: base}, nil
		}
		cmd.QtyTarget = &v
	}
	if req.Deadline != nil {
		v, base := dateField("deadline", req.GetDeadline())
		if base != nil {
			return &ppcv1.UpdateWorkOrderResponse{Base: base}, nil
		}
		cmd.Deadline = &v
	}

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateWorkOrderResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateWorkOrderResponse{Base: successResponse("Work order updated successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// DeleteWorkOrder deletes a WO.
func (h *workOrderHandler) DeleteWorkOrder(ctx context.Context, req *ppcv1.DeleteWorkOrderRequest) (*ppcv1.DeleteWorkOrderResponse, error) {
	if err := h.svc.Delete(ctx, req.GetWoId()); err != nil {
		return &ppcv1.DeleteWorkOrderResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteWorkOrderResponse{Base: successResponse("Work order deleted successfully")}, nil
}

// ListWorkOrders lists WOs with filtering and pagination.
func (h *workOrderHandler) ListWorkOrders(ctx context.Context, req *ppcv1.ListWorkOrdersRequest) (*ppcv1.ListWorkOrdersResponse, error) {
	result, err := h.svc.List(ctx, workorderapp.ListQuery{
		Page:       int(req.GetPage()),
		PageSize:   int(req.GetPageSize()),
		Search:     req.GetSearch(),
		Area:       areaCodeToString(req.GetArea()),
		Status:     woStatusToString(req.GetStatus()),
		MachineID:  req.MachineId,
		PlanItemID: req.PlanItemId,
		LotNo:      req.GetLotNo(),
		SortBy:     req.GetSortBy(),
		SortOrder:  req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListWorkOrdersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.WorkOrder, len(result.Items))
	for i, e := range result.Items {
		data[i] = workOrderToProto(e, nil)
	}
	return &ppcv1.ListWorkOrdersResponse{
		Base:       successResponse("Work orders retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ResolveWOParameters previews the resolved parameter values for a product+machine.
func (h *workOrderHandler) ResolveWOParameters(ctx context.Context, req *ppcv1.ResolveWOParametersRequest) (*ppcv1.ResolveWOParametersResponse, error) {
	resolved, err := h.svc.ResolveParameters(ctx, workorderapp.ResolveQuery{
		ProductSysID: req.GetCpmProductSysId(),
		MachineID:    req.GetMachineId(),
		RefWoID:      req.RefWoId,
		DisplayGroup: req.GetDisplayGroup(),
	})
	if err != nil {
		return &ppcv1.ResolveWOParametersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.ResolvedParam, len(resolved))
	for i, rp := range resolved {
		data[i] = resolvedParamToProto(rp)
	}
	return &ppcv1.ResolveWOParametersResponse{Base: successResponse("Parameters resolved successfully"), Data: data}, nil
}

// SaveWOParameters applies PPC parameter value edits.
func (h *workOrderHandler) SaveWOParameters(ctx context.Context, req *ppcv1.SaveWOParametersRequest) (*ppcv1.SaveWOParametersResponse, error) {
	values, base := paramValueInputs(req.GetPpcValues())
	if base != nil {
		return &ppcv1.SaveWOParametersResponse{Base: base}, nil
	}
	entity, err := h.svc.SaveWOParameters(ctx, workorderapp.SaveWOParametersCommand{WOID: req.GetWoId(), Values: values})
	if err != nil {
		return &ppcv1.SaveWOParametersResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SaveWOParametersResponse{Base: successResponse("Work order parameters saved successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// SaveWORmAllocations replaces a WO's RM allocation lines.
func (h *workOrderHandler) SaveWORmAllocations(ctx context.Context, req *ppcv1.SaveWORmAllocationsRequest) (*ppcv1.SaveWORmAllocationsResponse, error) {
	allocs, base := rmAllocationInputs(req.GetAllocations())
	if base != nil {
		return &ppcv1.SaveWORmAllocationsResponse{Base: base}, nil
	}
	result, err := h.svc.SaveWORmAllocations(ctx, workorderapp.SaveWORmAllocationsCommand{WOID: req.GetWoId(), Allocations: allocs})
	if err != nil {
		return &ppcv1.SaveWORmAllocationsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.WORmAllocation, len(result))
	for i, a := range result {
		data[i] = rmAllocationToProto(a)
	}
	return &ppcv1.SaveWORmAllocationsResponse{Base: successResponse("RM allocations saved successfully"), Data: data}, nil
}

// PopulateWORmFromRoute auto-materializes RM allocation suggestions from the WO's route.
func (h *workOrderHandler) PopulateWORmFromRoute(ctx context.Context, req *ppcv1.PopulateWORmFromRouteRequest) (*ppcv1.PopulateWORmFromRouteResponse, error) {
	result, err := h.svc.PopulateRmFromRoute(ctx, workorderapp.PopulateRmFromRouteCommand{WOID: req.GetWoId(), Replace: req.GetReplace()})
	if err != nil {
		return &ppcv1.PopulateWORmFromRouteResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.WORmAllocation, len(result))
	for i, a := range result {
		data[i] = rmAllocationToProto(a)
	}
	return &ppcv1.PopulateWORmFromRouteResponse{Base: successResponse("RM allocations populated from route successfully"), Data: data}, nil
}

// SaveWOExecution upserts one actual parameter value.
func (h *workOrderHandler) SaveWOExecution(ctx context.Context, req *ppcv1.SaveWOExecutionRequest) (*ppcv1.SaveWOExecutionResponse, error) {
	date, base := dateField("date", req.GetDate())
	if base != nil {
		return &ppcv1.SaveWOExecutionResponse{Base: base}, nil
	}
	num, base := optionalDecimalField("value_num", req.GetValueNum())
	if base != nil {
		return &ppcv1.SaveWOExecutionResponse{Base: base}, nil
	}
	exec, err := h.svc.SaveWOExecution(ctx, workorderapp.SaveWOExecutionCommand{
		WOID:    req.GetWoId(),
		Date:    date,
		Shift:   req.GetShift(),
		ParamID: req.GetParamId(),
		Num:     num,
		Text:    optionalStringField(req.GetValueText()),
		Flag:    optionalBoolField(req.GetValueFlag(), req.GetHasValueFlag()),
		InputBy: actorID(ctx),
	})
	if err != nil {
		return &ppcv1.SaveWOExecutionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SaveWOExecutionResponse{Base: successResponse("Work order execution saved successfully"), Data: executionToProto(exec)}, nil
}

// ListWOExecutions lists a WO's actual parameter values.
func (h *workOrderHandler) ListWOExecutions(ctx context.Context, req *ppcv1.ListWOExecutionsRequest) (*ppcv1.ListWOExecutionsResponse, error) {
	execs, err := h.svc.ListExecutions(ctx, req.GetWoId())
	if err != nil {
		return &ppcv1.ListWOExecutionsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.WOExecution, len(execs))
	for i, e := range execs {
		data[i] = executionToProto(e)
	}
	return &ppcv1.ListWOExecutionsResponse{Base: successResponse("Work order executions retrieved successfully"), Data: data}, nil
}

// SubmitWO submits a DRAFT WO for sequential PC→PM approval.
func (h *workOrderHandler) SubmitWO(ctx context.Context, req *ppcv1.SubmitWORequest) (*ppcv1.SubmitWOResponse, error) {
	values, base := paramValueInputs(req.GetPpcValues())
	if base != nil {
		return &ppcv1.SubmitWOResponse{Base: base}, nil
	}
	entity, err := h.svc.Submit(ctx, req.GetWoId(), values)
	if err != nil {
		return &ppcv1.SubmitWOResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.SubmitWOResponse{Base: successResponse("Work order submitted successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// ApproveWOParameter records the PC approval (fills PC values → PC_APPROVED).
func (h *workOrderHandler) ApproveWOParameter(ctx context.Context, req *ppcv1.ApproveWOParameterRequest) (*ppcv1.ApproveWOParameterResponse, error) {
	values, base := paramValueInputs(req.GetPcValues())
	if base != nil {
		return &ppcv1.ApproveWOParameterResponse{Base: base}, nil
	}
	entity, err := h.svc.ApproveWOParameter(ctx, req.GetWoId(), values, actorID(ctx))
	if err != nil {
		return &ppcv1.ApproveWOParameterResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ApproveWOParameterResponse{Base: successResponse("Work order parameters approved successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// ApproveWO records an approval side (PC/PM); PM approval finalizes the WO.
func (h *workOrderHandler) ApproveWO(ctx context.Context, req *ppcv1.ApproveWORequest) (*ppcv1.ApproveWOResponse, error) {
	entity, err := h.svc.ApproveWO(ctx, req.GetWoId(), req.GetApprovalSide(), actorID(ctx))
	if err != nil {
		return &ppcv1.ApproveWOResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ApproveWOResponse{Base: successResponse("Work order approval recorded successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// RejectWO sends a submitted WO back to PPC.
func (h *workOrderHandler) RejectWO(ctx context.Context, req *ppcv1.RejectWORequest) (*ppcv1.RejectWOResponse, error) {
	entity, err := h.svc.Reject(ctx, req.GetWoId(), req.GetReason())
	if err != nil {
		return &ppcv1.RejectWOResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.RejectWOResponse{Base: successResponse("Work order rejected successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// CreateWOReference creates a duplicate (TEMPLATE) or continuation WO.
func (h *workOrderHandler) CreateWOReference(ctx context.Context, req *ppcv1.CreateWOReferenceRequest) (*ppcv1.CreateWOReferenceResponse, error) {
	qty, base := decimalField("qty_target", req.GetQtyTarget())
	if base != nil {
		return &ppcv1.CreateWOReferenceResponse{Base: base}, nil
	}
	deadline, base := dateField("deadline", req.GetDeadline())
	if base != nil {
		return &ppcv1.CreateWOReferenceResponse{Base: base}, nil
	}
	entity, err := h.svc.CreateWOReference(ctx, workorderapp.CreateWOReferenceCommand{
		SourceWOID: req.GetSourceWoId(),
		RefType:    refTypeToString(req.GetRefType()),
		LotNo:      req.GetLotNo(),
		QtyTarget:  qty,
		Deadline:   deadline,
		MachineID:  req.MachineId,
		CreatedBy:  actorID(ctx),
	})
	if err != nil {
		return &ppcv1.CreateWOReferenceResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateWOReferenceResponse{Base: successResponse("Work order reference created successfully"), Data: workOrderToProto(entity, nil)}, nil
}

// GetWOProductionActual lists production-actual rows for a WO.
func (h *workOrderHandler) GetWOProductionActual(ctx context.Context, req *ppcv1.GetWOProductionActualRequest) (*ppcv1.GetWOProductionActualResponse, error) {
	date, base := optionalDateField("date", req.GetDate())
	if base != nil {
		return &ppcv1.GetWOProductionActualResponse{Base: base}, nil
	}
	actuals, err := h.svc.GetProductionActuals(ctx, req.GetWoId(), date, req.GetShift())
	if err != nil {
		return &ppcv1.GetWOProductionActualResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := make([]*ppcv1.WOProductionActual, len(actuals))
	for i, a := range actuals {
		data[i] = productionActualToProto(a)
	}
	return &ppcv1.GetWOProductionActualResponse{Base: successResponse("Production actuals retrieved successfully"), Data: data}, nil
}

// AdjustWOActual sets qty_actual (source=ADJUSTED) for a (wo,date,shift) row.
func (h *workOrderHandler) AdjustWOActual(ctx context.Context, req *ppcv1.AdjustWOActualRequest) (*ppcv1.AdjustWOActualResponse, error) {
	date, base := dateField("date", req.GetDate())
	if base != nil {
		return &ppcv1.AdjustWOActualResponse{Base: base}, nil
	}
	qty, base := decimalField("qty_actual", req.GetQtyActual())
	if base != nil {
		return &ppcv1.AdjustWOActualResponse{Base: base}, nil
	}
	actual, err := h.svc.AdjustWOActual(ctx, workorderapp.AdjustWOActualCommand{
		WOID:      req.GetWoId(),
		Date:      date,
		Shift:     req.GetShift(),
		QtyActual: qty,
		Reason:    req.GetReason(),
		EditedBy:  actorID(ctx),
	})
	if err != nil {
		return &ppcv1.AdjustWOActualResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.AdjustWOActualResponse{Base: successResponse("Work order actual adjusted successfully"), Data: productionActualToProto(actual)}, nil
}

// paramValueInputs maps proto param-value inputs to application inputs, parsing
// decimal-as-string values.
func paramValueInputs(in []*ppcv1.WOParamValueInput) ([]workorderapp.ParamValueInput, *commonv1.BaseResponse) {
	out := make([]workorderapp.ParamValueInput, 0, len(in))
	for _, v := range in {
		num, base := optionalDecimalField("value_num", v.GetValueNum())
		if base != nil {
			return nil, base
		}
		out = append(out, workorderapp.ParamValueInput{
			ParamID: v.GetParamId(),
			Num:     num,
			Text:    optionalStringField(v.GetValueText()),
			Flag:    optionalBoolField(v.GetValueFlag(), v.GetHasValueFlag()),
			HasFlag: v.GetHasValueFlag(),
		})
	}
	return out, nil
}

// rmAllocationInputs maps proto RM allocation inputs to application inputs.
func rmAllocationInputs(in []*ppcv1.WORmAllocationInput) ([]workorderapp.RmAllocationInput, *commonv1.BaseResponse) {
	out := make([]workorderapp.RmAllocationInput, 0, len(in))
	for _, a := range in {
		qty, base := decimalField("qty_allocated", a.GetQtyAllocated())
		if base != nil {
			return nil, base
		}
		out = append(out, workorderapp.RmAllocationInput{
			CrmRmID:      a.GetCrmRmId(),
			RmType:       a.GetRmType(),
			LotNo:        a.GetLotNo(),
			RmSource:     rmSourceToString(a.GetRmSource()),
			FreshBox:     a.GetFreshBox(),
			ShadeCode:    a.GetShadeCode(),
			QtyAllocated: qty,
			Notes:        a.GetNotes(),
		})
	}
	return out, nil
}
