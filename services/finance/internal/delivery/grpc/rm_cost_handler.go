package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	rmgroupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// RMCostHandler implements the RMCostServiceServer interface.
type RMCostHandler struct {
	financev1.UnimplementedRMCostServiceServer
	triggerHandler    *apprmcost.TriggerHandler
	calculateHandler  *apprmcost.CalculateHandler
	getHandler        *apprmcost.GetHandler
	listHandler       *apprmcost.ListHandler
	historyHandler    *apprmcost.HistoryHandler
	periodsHandler    *apprmcost.PeriodsHandler
	exportHandler     *apprmcost.ExportHandler
	costDetailRepo    rmcostdomain.CostDetailRepository
	editInputsHandler *apprmcost.EditInputsHandler
	editFixRateHandler *apprmcost.EditFixRateHandler
	validationHelper  *ValidationHelper
}

// NewRMCostHandler builds an RMCostHandler.
func NewRMCostHandler(
	trigger *apprmcost.TriggerHandler,
	calculate *apprmcost.CalculateHandler,
	get *apprmcost.GetHandler,
	list *apprmcost.ListHandler,
	history *apprmcost.HistoryHandler,
	periods *apprmcost.PeriodsHandler,
	export *apprmcost.ExportHandler,
	costDetailRepo rmcostdomain.CostDetailRepository,
	editInputs *apprmcost.EditInputsHandler,
	editFixRate *apprmcost.EditFixRateHandler,
) (*RMCostHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &RMCostHandler{
		triggerHandler:     trigger,
		calculateHandler:   calculate,
		getHandler:         get,
		listHandler:        list,
		historyHandler:     history,
		periodsHandler:     periods,
		exportHandler:      export,
		costDetailRepo:     costDetailRepo,
		editInputsHandler:  editInputs,
		editFixRateHandler: editFixRate,
		validationHelper:   v,
	}, nil
}

// TriggerRMCostCalculation enqueues an async calculation job.
func (h *RMCostHandler) TriggerRMCostCalculation(ctx context.Context, req *financev1.TriggerRMCostCalculationRequest) (*financev1.TriggerRMCostCalculationResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opTrigger, false)
		return &financev1.TriggerRMCostCalculationResponse{Base: baseResp}, nil
	}

	cmd := apprmcost.TriggerCommand{
		Period:    req.Period,
		Reason:    apprmcost.TriggerReason(triggerReasonToString(req.TriggerReason)),
		CreatedBy: getUserFromContext(ctx),
	}
	if gid, badResp := parseOptionalGroupHeadID(req.GroupHeadId); badResp != nil {
		RecordRMCostOperation(opTrigger, false)
		return &financev1.TriggerRMCostCalculationResponse{Base: badResp}, nil
	} else if gid != nil {
		cmd.GroupHeadID = gid
	}

	result, err := h.triggerHandler.Handle(ctx, cmd)
	if err != nil {
		RecordRMCostOperation(opTrigger, false)
		return &financev1.TriggerRMCostCalculationResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCostOperation(opTrigger, true)
	return &financev1.TriggerRMCostCalculationResponse{
		Base:  successResponse("RM cost calculation enqueued"),
		JobId: result.Execution.ID().String(),
	}, nil
}

// CalculateRMCost runs the calculation synchronously.
func (h *RMCostHandler) CalculateRMCost(ctx context.Context, req *financev1.CalculateRMCostRequest) (*financev1.CalculateRMCostResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opCalculate, false)
		return &financev1.CalculateRMCostResponse{Base: baseResp}, nil
	}

	cmd := apprmcost.CalculateCommand{
		Period:        req.Period,
		TriggerReason: rmcostdomain.HistoryTriggerReason(triggerReasonToString(req.TriggerReason)),
		CalculatedBy:  getUserFromContext(ctx),
	}
	if gid, badResp := parseOptionalGroupHeadID(req.GroupHeadId); badResp != nil {
		RecordRMCostOperation(opCalculate, false)
		return &financev1.CalculateRMCostResponse{Base: badResp}, nil
	} else if gid != nil {
		cmd.GroupHeadID = gid
	}

	result, err := h.calculateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordRMCostOperation(opCalculate, false)
		return &financev1.CalculateRMCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCostOperation(opCalculate, true)
	return &financev1.CalculateRMCostResponse{
		Base:      successResponse("RM cost calculated"),
		Processed: safeIntToInt32(result.Processed),
		Skipped:   safeIntToInt32(result.Skipped),
		Period:    result.Period,
	}, nil
}

// GetRMCost fetches a single cost row by (period, rm_code).
func (h *RMCostHandler) GetRMCost(ctx context.Context, req *financev1.GetRMCostRequest) (*financev1.GetRMCostResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opGet, false)
		return &financev1.GetRMCostResponse{Base: baseResp}, nil
	}

	cost, err := h.getHandler.Handle(ctx, apprmcost.GetQuery{
		Period: req.Period,
		RMCode: req.RmCode,
	})
	if err != nil {
		RecordRMCostOperation(opGet, false)
		return &financev1.GetRMCostResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCostOperation(opGet, true)
	return &financev1.GetRMCostResponse{
		Base: successResponse(msgRMCostRetrieved),
		Data: rmCostToProto(cost),
	}, nil
}

// ListRMCosts returns a paginated list of cost rows.
func (h *RMCostHandler) ListRMCosts(ctx context.Context, req *financev1.ListRMCostsRequest) (*financev1.ListRMCostsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opList, false)
		return &financev1.ListRMCostsResponse{Base: baseResp}, nil
	}

	query := apprmcost.ListQuery{
		Page:      int(req.Page),
		PageSize:  int(req.PageSize),
		Period:    req.Period,
		RMType:    rmTypeToString(req.RmType),
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}
	if req.GroupHeadId != nil {
		query.GroupHeadID = *req.GroupHeadId
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordRMCostOperation(opList, false)
		return &financev1.ListRMCostsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.RMCost, len(result.Costs))
	for i, c := range result.Costs {
		items[i] = rmCostToProto(c)
	}

	RecordRMCostOperation(opList, true)
	return &financev1.ListRMCostsResponse{
		Base: successResponse("RM costs retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ListRMCostHistory returns a paginated list of history rows.
func (h *RMCostHandler) ListRMCostHistory(ctx context.Context, req *financev1.ListRMCostHistoryRequest) (*financev1.ListRMCostHistoryResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opListHistory, false)
		return &financev1.ListRMCostHistoryResponse{Base: baseResp}, nil
	}

	query := apprmcost.HistoryQuery{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
		Period:   req.Period,
		RMCode:   req.RmCode,
	}
	if req.GroupHeadId != nil {
		query.GroupHeadID = *req.GroupHeadId
	}
	if req.JobId != nil {
		query.JobID = *req.JobId
	}

	result, err := h.historyHandler.Handle(ctx, query)
	if err != nil {
		RecordRMCostOperation(opListHistory, false)
		return &financev1.ListRMCostHistoryResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.RMCostHistory, len(result.Rows))
	for i := range result.Rows {
		items[i] = rmCostHistoryToProto(&result.Rows[i])
	}

	RecordRMCostOperation(opListHistory, true)
	return &financev1.ListRMCostHistoryResponse{
		Base: successResponse("RM cost history retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ListRMCostPeriods returns distinct periods from cost rows.
func (h *RMCostHandler) ListRMCostPeriods(ctx context.Context, _ *financev1.ListRMCostPeriodsRequest) (*financev1.ListRMCostPeriodsResponse, error) {
	periods, err := h.periodsHandler.Handle(ctx)
	if err != nil {
		return &financev1.ListRMCostPeriodsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	return &financev1.ListRMCostPeriodsResponse{
		Base:    successResponse("RM cost periods retrieved successfully"),
		Periods: periods,
	}, nil
}

// ExportRMCosts exports cost rows matching the filter to a single-sheet Excel.
func (h *RMCostHandler) ExportRMCosts(ctx context.Context, req *financev1.ExportRMCostsRequest) (*financev1.ExportRMCostsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMCostOperation(opExport, false)
		return &financev1.ExportRMCostsResponse{Base: baseResp}, nil
	}

	query := apprmcost.ExportQuery{
		Period: req.Period,
		RMType: rmcostdomain.RMType(rmTypeToString(req.RmType)),
		Search: req.Search,
	}
	if gid, badResp := parseOptionalGroupHeadID(req.GroupHeadId); badResp != nil {
		RecordRMCostOperation(opExport, false)
		return &financev1.ExportRMCostsResponse{Base: badResp}, nil
	} else if gid != nil {
		query.GroupHeadID = gid
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordRMCostOperation(opExport, false)
		return &financev1.ExportRMCostsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMCostOperation(opExport, true)
	return &financev1.ExportRMCostsResponse{
		Base:        successResponse("RM costs exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// =============================================================================
// Entity → proto mappers
// =============================================================================

func rmCostToProto(c *rmcostdomain.Cost) *financev1.RMCost {
	out := &financev1.RMCost{
		RmCostId:           c.ID().String(),
		Period:             c.Period(),
		RmCode:             c.RMCode(),
		RmType:             rmTypeToProto(c.RMType()),
		RmName:             c.RMName(),
		UomCode:            c.UOMCode(),
		Rates:              rmCostRatesToProto(c.Rates()),
		CostValuation:      c.CostValuation(),
		CostMarketing:      c.CostMarketing(),
		CostSimulation:     c.CostSimulation(),
		FlagValuation:      stageToProto(c.FlagValuation()),
		FlagMarketing:      stageToProto(c.FlagMarketing()),
		FlagSimulation:     stageToProto(c.FlagSimulation()),
		FlagValuationUsed:  stageToProto(c.FlagValuationUsed()),
		FlagMarketingUsed:  stageToProto(c.FlagMarketingUsed()),
		FlagSimulationUsed: stageToProto(c.FlagSimulationUsed()),
		Audit: &commonv1.AuditInfo{
			CreatedAt: c.CreatedAt().Format(time.RFC3339),
			CreatedBy: c.CreatedBy(),
		},
	}
	if id := c.GroupHeadID(); id != nil {
		s := id.String()
		out.GroupHeadId = &s
	}
	if code := c.ItemCode(); code != nil {
		s := *code
		out.ItemCode = &s
	}
	if t := c.CalculatedAt(); t != nil {
		out.CalculatedAt = t.Format(time.RFC3339)
	}
	if by := c.CalculatedBy(); by != nil {
		out.CalculatedBy = *by
	}
	if t := c.UpdatedAt(); t != nil {
		out.Audit.UpdatedAt = t.Format(time.RFC3339)
	}
	if by := c.UpdatedBy(); by != nil {
		out.Audit.UpdatedBy = *by
	}
	// V2 fields.
	if v2 := c.V2Inputs(); v2 != nil {
		out.MarketingFreightRate = v2.MarketingFreightRate
		out.MarketingAntiDumpingPct = v2.MarketingAntiDumpingPct
		out.MarketingDutyPct = v2.MarketingDutyPct
		out.MarketingTransportRate = v2.MarketingTransportRate
		out.MarketingDefaultValue = v2.MarketingDefaultValue
		out.SimulationRate = v2.SimulationRate
		out.ValuationFlag = parseProtoValuationFlag(v2.ValuationFlag)
		out.MarketingFlag = parseProtoMarketingFlag(v2.MarketingFlag)
	}
	if r := c.V2Rates(); r != nil {
		out.ClRate = r.CL
		out.SlRate = r.SL
		out.FlRate = r.FL
		out.SpRate = r.SP
		out.PpRate = r.PP
		out.FpRate = r.FP
		out.CrRate = r.CR
		out.SrRate = r.SR
		out.PrRate = r.PR
	}
	return out
}

// parseProtoValuationFlag maps the string form ("AUTO"/"CR"/...) to the proto enum.
func parseProtoValuationFlag(s string) financev1.RMValuationFlag {
	switch s {
	case "CR":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_CR
	case "SR":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_SR
	case "PR":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_PR
	case "CL":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_CL
	case "SL":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_SL
	case "FL":
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_FL
	default:
		return financev1.RMValuationFlag_RM_VALUATION_FLAG_UNSPECIFIED
	}
}

// parseProtoMarketingFlag maps "AUTO"/"SP"/... to the proto enum.
func parseProtoMarketingFlag(s string) financev1.RMMarketingFlag {
	switch s {
	case "SP":
		return financev1.RMMarketingFlag_RM_MARKETING_FLAG_SP
	case "PP":
		return financev1.RMMarketingFlag_RM_MARKETING_FLAG_PP
	case "FP":
		return financev1.RMMarketingFlag_RM_MARKETING_FLAG_FP
	default:
		return financev1.RMMarketingFlag_RM_MARKETING_FLAG_UNSPECIFIED
	}
}

func rmCostHistoryToProto(h *rmcostdomain.History) *financev1.RMCostHistory {
	out := &financev1.RMCostHistory{
		HistoryId:          h.ID.String(),
		Period:             h.Period,
		RmCode:             h.RMCode,
		RmType:             rmTypeToProto(h.RMType),
		Rates:              rmCostRatesToProto(h.Rates),
		CostPercentage:     h.CostPercentage,
		CostPerKg:          h.CostPerKg,
		FlagValuation:      stageToProto(h.FlagValuation),
		FlagMarketing:      stageToProto(h.FlagMarketing),
		FlagSimulation:     stageToProto(h.FlagSimulation),
		InitValValuation:   h.InitValValuation,
		InitValMarketing:   h.InitValMarketing,
		InitValSimulation:  h.InitValSimulation,
		CostValuation:      h.CostValuation,
		CostMarketing:      h.CostMarketing,
		CostSimulation:     h.CostSimulation,
		FlagValuationUsed:  stageToProto(h.FlagValuationUsed),
		FlagMarketingUsed:  stageToProto(h.FlagMarketingUsed),
		FlagSimulationUsed: stageToProto(h.FlagSimulationUsed),
		SourceItemCount:    safeIntToInt32(h.SourceItemCount),
		TriggerReason:      triggerReasonToProto(h.TriggerReason),
		CalculatedAt:       h.CalculatedAt.Format(time.RFC3339),
		CalculatedBy:       h.CalculatedBy,
	}
	if h.RMCostID != nil {
		s := h.RMCostID.String()
		out.RmCostId = &s
	}
	if h.JobID != nil {
		s := h.JobID.String()
		out.JobId = &s
	}
	if h.GroupHeadID != nil {
		s := h.GroupHeadID.String()
		out.GroupHeadId = &s
	}
	return out
}

func rmCostRatesToProto(r rmcostdomain.StageRates) *financev1.RMCostRates {
	return &financev1.RMCostRates{
		Cons:   r.Cons,
		Stores: r.Stores,
		Dept:   r.Dept,
		Po_1:   r.PO1,
		Po_2:   r.PO2,
		Po_3:   r.PO3,
	}
}

// stageToProto reuses flagToProto by casting Stage (same underlying string domain).
func stageToProto(s rmcostdomain.Stage) financev1.RMGroupFlag {
	return flagToProto(rmgroupdomain.Flag(s))
}

func rmTypeToProto(t rmcostdomain.RMType) financev1.RMCostType {
	switch t {
	case rmcostdomain.RMTypeGroup:
		return financev1.RMCostType_RM_COST_TYPE_GROUP
	case rmcostdomain.RMTypeItem:
		return financev1.RMCostType_RM_COST_TYPE_ITEM
	default:
		return financev1.RMCostType_RM_COST_TYPE_UNSPECIFIED
	}
}

func rmTypeToString(t financev1.RMCostType) string {
	switch t {
	case financev1.RMCostType_RM_COST_TYPE_GROUP:
		return string(rmcostdomain.RMTypeGroup)
	case financev1.RMCostType_RM_COST_TYPE_ITEM:
		return string(rmcostdomain.RMTypeItem)
	case financev1.RMCostType_RM_COST_TYPE_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func triggerReasonToString(r financev1.RMCostTriggerReason) string {
	switch r {
	case financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_ORACLE_SYNC_CHAIN:
		return string(rmcostdomain.TriggerOracleSyncChain)
	case financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_GROUP_UPDATE:
		return string(rmcostdomain.TriggerGroupUpdate)
	case financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_DETAIL_CHANGE:
		return string(rmcostdomain.TriggerDetailChange)
	case financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_MANUAL_UI:
		return string(rmcostdomain.TriggerManualUI)
	case financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func triggerReasonToProto(r rmcostdomain.HistoryTriggerReason) financev1.RMCostTriggerReason {
	switch r {
	case rmcostdomain.TriggerOracleSyncChain:
		return financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_ORACLE_SYNC_CHAIN
	case rmcostdomain.TriggerGroupUpdate:
		return financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_GROUP_UPDATE
	case rmcostdomain.TriggerDetailChange:
		return financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_DETAIL_CHANGE
	case rmcostdomain.TriggerManualUI:
		return financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_MANUAL_UI
	default:
		return financev1.RMCostTriggerReason_RM_COST_TRIGGER_REASON_UNSPECIFIED
	}
}

// RecordRMCostOperation records an RM Cost operation metric.
func RecordRMCostOperation(operation string, success bool) {
	rmCostOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}

// parseOptionalGroupHeadID parses an optional *string group_head_id into a
// *uuid.UUID. Returns (nil, nil) when the input is nil or empty. Returns
// (nil, baseResponse) when the input is non-empty but invalid.
func parseOptionalGroupHeadID(raw *string) (*uuid.UUID, *commonv1.BaseResponse) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, ErrorResponse("400", "invalid group_head_id: "+err.Error())
	}
	return &id, nil
}
