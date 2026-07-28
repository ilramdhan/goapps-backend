// Package grpc provides gRPC server implementation for the PPC service.
package grpc

import (
	"context"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// demandHandler implements the Layer-1 demand RPCs of PPCService.
type demandHandler struct {
	svc *demandapp.Service
}

func newDemandHandler(svc *demandapp.Service) *demandHandler {
	return &demandHandler{svc: svc}
}

// CreateDemand creates a new production demand.
func (h *demandHandler) CreateDemand(ctx context.Context, req *ppcv1.CreateDemandRequest) (*ppcv1.CreateDemandResponse, error) {
	qty, base := decimalField("qty_original", req.GetQtyOriginal())
	if base != nil {
		return &ppcv1.CreateDemandResponse{Base: base}, nil
	}
	deadline, base := dateField("deadline", req.GetDeadline())
	if base != nil {
		return &ppcv1.CreateDemandResponse{Base: base}, nil
	}
	axMin, base := optionalDecimalField("ax_min_pct", req.GetAxMinPct())
	if base != nil {
		return &ppcv1.CreateDemandResponse{Base: base}, nil
	}
	amMax, base := optionalDecimalField("am_max_pct", req.GetAmMaxPct())
	if base != nil {
		return &ppcv1.CreateDemandResponse{Base: base}, nil
	}
	contractDate, base := optionalDateField("contract_date", req.GetContractDate())
	if base != nil {
		return &ppcv1.CreateDemandResponse{Base: base}, nil
	}

	entity, err := h.svc.Create(ctx, demandapp.CreateCommand{
		Type:            demandTypeToString(req.GetType()),
		SubType:         demandSubTypeToString(req.GetSubType()),
		Source:          demandSourceToString(req.GetSource()),
		CpmProductSysID: req.GetCpmProductSysId(),
		QtyOriginal:     qty,
		Deadline:        deadline,
		GradeReq:        gradeReqToString(req.GetGradeRequirement()),
		AxMinPct:        axMin,
		AmMaxPct:        amMax,
		SosRef:          req.SosRef,
		CustomerID:      req.CustomerId,
		ContractNo:      req.GetContractNo(),
		ContractDate:    contractDate,
		Incoterm:        req.GetIncoterm(),
		LcStatus:        req.GetLcStatus(),
		StuffAdvanceNo:  req.GetStuffAdvanceNo(),
		Month:           req.GetMonth(),
		MonthOverride:   req.GetMonthOverride(),
		CreatedBy:       actorID(ctx),
	})
	if err != nil {
		return &ppcv1.CreateDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.CreateDemandResponse{Base: successResponse("Demand created successfully"), Data: h.decorateDemand(ctx, entity)}, nil
}

// GetDemand retrieves a demand by ID.
func (h *demandHandler) GetDemand(ctx context.Context, req *ppcv1.GetDemandRequest) (*ppcv1.GetDemandResponse, error) {
	entity, err := h.svc.Get(ctx, req.GetDemandId())
	if err != nil {
		return &ppcv1.GetDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.GetDemandResponse{Base: successResponse("Demand retrieved successfully"), Data: h.decorateDemand(ctx, entity)}, nil
}

// UpdateDemand updates an existing demand.
func (h *demandHandler) UpdateDemand(ctx context.Context, req *ppcv1.UpdateDemandRequest) (*ppcv1.UpdateDemandResponse, error) {
	cmd := demandapp.UpdateCommand{
		ID:             req.GetDemandId(),
		ContractNo:     req.ContractNo,
		Incoterm:       req.Incoterm,
		LcStatus:       req.LcStatus,
		StuffAdvanceNo: req.StuffAdvanceNo,
	}
	if req.QtyOriginal != nil {
		v, base := decimalField("qty_original", req.GetQtyOriginal())
		if base != nil {
			return &ppcv1.UpdateDemandResponse{Base: base}, nil
		}
		cmd.QtyOriginal = &v
	}
	if req.Deadline != nil {
		v, base := dateField("deadline", req.GetDeadline())
		if base != nil {
			return &ppcv1.UpdateDemandResponse{Base: base}, nil
		}
		cmd.Deadline = &v
	}
	if req.GradeRequirement != nil {
		g := gradeReqToString(req.GetGradeRequirement())
		cmd.GradeReq = &g
	}
	if req.AxMinPct != nil {
		v, base := optionalDecimalField("ax_min_pct", req.GetAxMinPct())
		if base != nil {
			return &ppcv1.UpdateDemandResponse{Base: base}, nil
		}
		cmd.AxMinPct = v
	}
	if req.AmMaxPct != nil {
		v, base := optionalDecimalField("am_max_pct", req.GetAmMaxPct())
		if base != nil {
			return &ppcv1.UpdateDemandResponse{Base: base}, nil
		}
		cmd.AmMaxPct = v
	}

	entity, err := h.svc.Update(ctx, cmd)
	if err != nil {
		return &ppcv1.UpdateDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.UpdateDemandResponse{Base: successResponse("Demand updated successfully"), Data: h.decorateDemand(ctx, entity)}, nil
}

// MapDemandProduct maps a product onto a demand that currently has none.
func (h *demandHandler) MapDemandProduct(ctx context.Context, req *ppcv1.MapDemandProductRequest) (*ppcv1.MapDemandProductResponse, error) {
	entity, err := h.svc.MapProduct(ctx, demandapp.MapProductCommand{
		ID:              req.GetDemandId(),
		CpmProductSysID: req.GetCpmProductSysId(),
	})
	if err != nil {
		return &ppcv1.MapDemandProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.MapDemandProductResponse{Base: successResponse("Product mapped successfully"), Data: h.decorateDemand(ctx, entity)}, nil
}

// DeleteDemand deletes a demand.
func (h *demandHandler) DeleteDemand(ctx context.Context, req *ppcv1.DeleteDemandRequest) (*ppcv1.DeleteDemandResponse, error) {
	if err := h.svc.Delete(ctx, req.GetDemandId()); err != nil {
		return &ppcv1.DeleteDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.DeleteDemandResponse{Base: successResponse("Demand deleted successfully")}, nil
}

// ListDemands lists demands with 3-tab filtering and pagination.
func (h *demandHandler) ListDemands(ctx context.Context, req *ppcv1.ListDemandsRequest) (*ppcv1.ListDemandsResponse, error) {
	result, err := h.svc.List(ctx, demandapp.ListQuery{
		Page:            int(req.GetPage()),
		PageSize:        int(req.GetPageSize()),
		Search:          req.GetSearch(),
		Type:            demandTypeToString(req.GetType()),
		Status:          demandStatusToString(req.GetStatus()),
		Month:           req.GetMonth(),
		CpmProductSysID: req.CpmProductSysId,
		WithoutPlan:     req.GetWithoutPlan(),
		SortBy:          req.GetSortBy(),
		SortOrder:       req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListDemandsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := h.decorateDemands(ctx, result.Items)
	return &ppcv1.ListDemandsResponse{
		Base:       successResponse("Demands retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ConfirmDemand confirms a pending demand.
func (h *demandHandler) ConfirmDemand(ctx context.Context, req *ppcv1.ConfirmDemandRequest) (*ppcv1.ConfirmDemandResponse, error) {
	entity, err := h.svc.Confirm(ctx, req.GetDemandId(), actorID(ctx))
	if err != nil {
		return &ppcv1.ConfirmDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ConfirmDemandResponse{Base: successResponse("Demand confirmed successfully"), Data: h.decorateDemand(ctx, entity)}, nil
}

// ApproveMTSDemand records the Marketing decision for an MTS demand.
func (h *demandHandler) ApproveMTSDemand(ctx context.Context, req *ppcv1.ApproveMTSDemandRequest) (*ppcv1.ApproveMTSDemandResponse, error) {
	entity, err := h.svc.ApproveMTS(ctx, req.GetDemandId(), req.GetApproved(), actorID(ctx))
	if err != nil {
		return &ppcv1.ApproveMTSDemandResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ApproveMTSDemandResponse{Base: successResponse("MTS demand decision recorded"), Data: h.decorateDemand(ctx, entity)}, nil
}

// PullFromOrion creates demands from selected SO staging rows.
func (h *demandHandler) PullFromOrion(ctx context.Context, req *ppcv1.PullFromOrionRequest) (*ppcv1.PullFromOrionResponse, error) {
	// req.month is ignored: each demand's month is derived from its own
	// staging deadline, so a batch spanning a month boundary stays correct.
	created, err := h.svc.PullFromOrion(ctx, demandapp.PullFromOrionCommand{
		SosIDs:    req.GetSosIds(),
		SubType:   demandSubTypeToString(req.GetSubType()),
		CreatedBy: actorID(ctx),
	})
	if err != nil {
		return &ppcv1.PullFromOrionResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := h.decorateDemands(ctx, created)
	return &ppcv1.PullFromOrionResponse{
		Base:         successResponse("Demands pulled from Orion successfully"),
		Data:         data,
		CreatedCount: int32(len(created)), //nolint:gosec // bounded by request max_items=200
	}, nil
}

// ListCarryForwardCandidates returns demands eligible for carry-forward.
func (h *demandHandler) ListCarryForwardCandidates(ctx context.Context, req *ppcv1.ListCarryForwardCandidatesRequest) (*ppcv1.ListCarryForwardCandidatesResponse, error) {
	items, err := h.svc.ListCarryCandidates(ctx, req.GetSourceMonth())
	if err != nil {
		return &ppcv1.ListCarryForwardCandidatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := h.decorateDemands(ctx, items)
	return &ppcv1.ListCarryForwardCandidatesResponse{Base: successResponse("Carry-forward candidates retrieved"), Data: data}, nil
}

// ProcessCarryForward executes a carry-forward action for a demand.
func (h *demandHandler) ProcessCarryForward(ctx context.Context, req *ppcv1.ProcessCarryForwardRequest) (*ppcv1.ProcessCarryForwardResponse, error) {
	cmd, base := buildCarryForwardCommand(ctx, req)
	if base != nil {
		return &ppcv1.ProcessCarryForwardResponse{Base: base}, nil
	}
	created, err := h.svc.ProcessCarryForward(ctx, cmd)
	if err != nil {
		return &ppcv1.ProcessCarryForwardResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	data := h.decorateDemands(ctx, created)
	return &ppcv1.ProcessCarryForwardResponse{Base: successResponse("Carry-forward processed successfully"), Data: data}, nil
}

// ListSalesOrderStaging lists the Orion SO staging inbox (LOV).
func (h *demandHandler) ListSalesOrderStaging(ctx context.Context, req *ppcv1.ListSalesOrderStagingRequest) (*ppcv1.ListSalesOrderStagingResponse, error) {
	result, err := h.svc.ListStaging(ctx, demandapp.StagingListQuery{
		Page:         int(req.GetPage()),
		PageSize:     int(req.GetPageSize()),
		Search:       req.GetSearch(),
		CustomerCode: req.GetCustomerCode(),
		ItemCode:     req.GetItemCode(),
		UnpulledOnly: req.GetUnpulledOnly(),
		SortBy:       req.GetSortBy(),
		SortOrder:    req.GetSortOrder(),
	})
	if err != nil {
		return &ppcv1.ListSalesOrderStagingResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	// Decorate resolved rows with product code/name in one batch finance call —
	// ppc_db and finance_db are separate databases, so this cannot be a join.
	products := h.svc.LookupProducts(ctx, resolvedStagingProductIDs(result.Items))
	data := make([]*ppcv1.SalesOrderStaging, len(result.Items))
	for i, e := range result.Items {
		data[i] = stagingToProto(e, products)
	}
	return &ppcv1.ListSalesOrderStagingResponse{
		Base:       successResponse("Sales order staging retrieved successfully"),
		Data:       data,
		Pagination: paginationProto(result.CurrentPage, result.PageSize, result.TotalItems, result.TotalPages),
	}, nil
}

// ListSalesOrderStagingIds returns the ids of the staging rows matching a
// filter, for the LOV's "select all matching". No product decoration: ids only.
//
//nolint:revive // name is fixed by the generated PpcServiceServer interface
func (h *demandHandler) ListSalesOrderStagingIds(ctx context.Context, req *ppcv1.ListSalesOrderStagingIdsRequest) (*ppcv1.ListSalesOrderStagingIdsResponse, error) {
	result, err := h.svc.ListStagingIDs(ctx, demandapp.StagingIDsQuery{
		Search:       req.GetSearch(),
		CustomerCode: req.GetCustomerCode(),
		ItemCode:     req.GetItemCode(),
		UnpulledOnly: req.GetUnpulledOnly(),
	})
	if err != nil {
		return &ppcv1.ListSalesOrderStagingIdsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &ppcv1.ListSalesOrderStagingIdsResponse{
		Base:         successResponse("Sales order staging ids retrieved successfully"),
		SosIds:       result.SosIDs,
		TotalMatched: result.TotalMatched,
		Limit:        safeconv.IntToInt32(result.Limit),
	}, nil
}

// SetStagingProduct persists a planner's manual product pick on a staging row.
func (h *demandHandler) SetStagingProduct(ctx context.Context, req *ppcv1.SetStagingProductRequest) (*ppcv1.SetStagingProductResponse, error) {
	row, err := h.svc.SetStagingProduct(ctx, demandapp.SetStagingProductCommand{
		SosID:           req.GetSosId(),
		CpmProductSysID: req.GetCpmProductSysId(),
	})
	if err != nil {
		return &ppcv1.SetStagingProductResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	products := h.svc.LookupProducts(ctx, resolvedStagingProductIDs([]*demanddomain.SalesOrderStaging{row}))
	return &ppcv1.SetStagingProductResponse{
		Base: successResponse("Staging product set successfully"),
		Data: stagingToProto(row, products),
	}, nil
}

// buildCarryForwardCommand decodes and validates a carry-forward request.
func buildCarryForwardCommand(ctx context.Context, req *ppcv1.ProcessCarryForwardRequest) (demandapp.ProcessCarryForwardCommand, *commonv1.BaseResponse) {
	cmd := demandapp.ProcessCarryForwardCommand{
		SourceDemandID: req.GetSourceDemandId(),
		Action:         carryActionToString(req.GetAction()),
		TargetMonth:    req.GetTargetMonth(),
		ActedBy:        actorID(ctx),
	}
	if req.NewDeadline != nil {
		v, base := dateField("new_deadline", req.GetNewDeadline())
		if base != nil {
			return cmd, base
		}
		cmd.NewDeadline = &v
	}
	if req.CarryQty != nil {
		v, base := decimalField("carry_qty", req.GetCarryQty())
		if base != nil {
			return cmd, base
		}
		cmd.CarryQty = &v
	}
	for _, sp := range req.GetSplits() {
		qty, base := decimalField("split.qty", sp.GetQty())
		if base != nil {
			return cmd, base
		}
		deadline, base := dateField("split.deadline", sp.GetDeadline())
		if base != nil {
			return cmd, base
		}
		cmd.Splits = append(cmd.Splits, demandapp.CarryForwardSplit{Qty: qty, Deadline: deadline})
	}
	return cmd, nil
}

// demandSysIDs collects the distinct product sys ids referenced by a set of
// demands, for a single batched product lookup. Unmapped demands (sys id 0)
// are skipped — there is nothing to look up.
func demandSysIDs(items []*demanddomain.Demand) []int64 {
	seen := make(map[int64]struct{}, len(items))
	ids := make([]int64, 0, len(items))
	for _, e := range items {
		id := e.CpmProductSysID()
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// decorateDemands maps a slice of demands to proto, resolving product
// code/name for the whole page in one batched finance call. ppc_db and
// finance_db are separate databases, so this cannot be a join.
func (h *demandHandler) decorateDemands(ctx context.Context, items []*demanddomain.Demand) []*ppcv1.Demand {
	products := h.svc.LookupProducts(ctx, demandSysIDs(items))
	orionCodes := h.svc.LookupOrionItemCodes(ctx, items)
	data := make([]*ppcv1.Demand, len(items))
	for i, e := range items {
		data[i] = demandToProto(e, products, orionCodes)
	}
	return data
}

// decorateDemand maps a single demand to proto with its product labels.
func (h *demandHandler) decorateDemand(ctx context.Context, e *demanddomain.Demand) *ppcv1.Demand {
	items := []*demanddomain.Demand{e}
	products := h.svc.LookupProducts(ctx, demandSysIDs(items))
	return demandToProto(e, products, h.svc.LookupOrionItemCodes(ctx, items))
}

func demandToProto(
	e *demanddomain.Demand,
	products map[int64]*financev1.CostMasterProduct,
	orionCodes map[int64]string,
) *ppcv1.Demand {
	proto := &ppcv1.Demand{
		DemandId:          e.ID(),
		Type:              stringToDemandType(e.Type()),
		SubType:           stringToDemandSubType(e.SubType()),
		Source:            stringToDemandSource(e.Source()),
		CarryAction:       stringToCarryAction(e.CarryAction()),
		CpmProductSysId:   e.CpmProductSysID(),
		QtyOriginal:       formatDecimal(e.QtyOriginal()),
		QtyRemaining:      formatDecimal(e.QtyRemaining()),
		Deadline:          formatDate(e.Deadline()),
		ContractNo:        e.ContractNo(),
		StuffAdvanceNo:    e.StuffAdvanceNo(),
		Incoterm:          e.Incoterm(),
		LcStatus:          e.LcStatus(),
		GradeRequirement:  stringToGradeReq(e.GradeReq()),
		AxMinPct:          formatOptionalDecimal(e.AxMinPct()),
		AmMaxPct:          formatOptionalDecimal(e.AmMaxPct()),
		Status:            stringToDemandStatus(e.Status()),
		Month:             e.Month(),
		EstProdNeeded:     formatDecimal(e.QtyRemaining()),
		ShadeCode:         e.ShadeCode(),
		ShadeName:         e.ShadeName(),
		ProductLinkReason: e.ProductLinkReason(),
		OrionItemCode:     orionCodes[e.ID()],
		Audit:             &commonv1.AuditInfo{CreatedBy: formatInt64(e.CreatedBy()), CreatedAt: e.CreatedAt().Format(time.RFC3339)},
	}
	if e.CustomerID() != nil {
		proto.CustomerId = *e.CustomerID()
	}
	if e.ContractDate() != nil {
		proto.ContractDate = formatDate(*e.ContractDate())
	}
	if e.CarryFromID() != nil {
		proto.CarryFromId = *e.CarryFromID()
	}
	if e.SosRef() != nil {
		proto.SosRef = *e.SosRef()
	}
	if e.ConfirmedBy() != nil {
		proto.ConfirmedBy = *e.ConfirmedBy()
	}
	if e.ConfirmedAt() != nil {
		proto.ConfirmedAt = e.ConfirmedAt().Format(time.RFC3339)
	}
	if p, ok := products[e.CpmProductSysID()]; ok {
		proto.ProductCode = p.GetProductCode()
		proto.ProductName = p.GetProductName()
	}
	return proto
}

// resolvedStagingProductIDs collects the distinct resolved product sys ids on a
// staging page, for the batch display decoration.
func resolvedStagingProductIDs(items []*demanddomain.SalesOrderStaging) []int64 {
	seen := make(map[int64]struct{}, len(items))
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		if item.CpmProductSysID == nil {
			continue
		}
		id := *item.CpmProductSysID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func stagingToProto(s *demanddomain.SalesOrderStaging, products map[int64]*financev1.CostMasterProduct) *ppcv1.SalesOrderStaging {
	proto := &ppcv1.SalesOrderStaging{
		MatchStatus:   s.MatchStatus,
		MatchCount:    s.MatchCount,
		SosId:         s.SosID,
		ContractNo:    s.ContractNo,
		CustomerCode:  s.CustomerCode,
		CustomerName:  s.CustomerName,
		ItemCode:      s.ItemCode,
		ItemDesc:      s.ItemDesc,
		GradeCode:     s.GradeCode,
		ShadeCode:     s.ShadeCode,
		ShadeName:     s.ShadeName,
		QtyOrdered:    formatDecimal(s.QtyOrdered),
		QtyDelivered:  formatDecimal(s.QtyDelivered),
		QtyRemaining:  formatDecimal(s.QtyRemaining),
		ShipDate:      s.ShipDate,
		MergeNo:       s.MergeNo,
		Term:          s.Term,
		Rate:          formatDecimal(s.Rate),
		Currency:      s.Currency,
		BlockedStatus: s.BlockedStatus,
		OutstandingAr: formatDecimal(s.OutstandingAr),
		PalletType:    s.PalletType,
		EndUse:        s.EndUse,
		MixFlag:       s.MixFlag,
		Annotation:    s.Annotation,
		Remarks:       s.Remarks,
	}
	if s.ContractDate != nil {
		proto.ContractDate = formatDate(*s.ContractDate)
	}
	if s.ContractSysID != nil {
		proto.ContractSysId = *s.ContractSysID
	}
	if s.Deadline != nil {
		proto.Deadline = formatDate(*s.Deadline)
	}
	if s.EtlSyncedAt != nil {
		proto.EtlSyncedAt = s.EtlSyncedAt.Format(time.RFC3339)
	}
	if s.PulledToDemandID != nil {
		proto.PulledToDemandId = *s.PulledToDemandID
	}
	if s.CpmProductSysID != nil {
		proto.CpmProductSysId = *s.CpmProductSysID
		if p, ok := products[*s.CpmProductSysID]; ok {
			proto.CpmProductCode = p.GetProductCode()
			proto.CpmProductName = p.GetProductName()
		}
	}
	return proto
}
