package grpc

import (
	"context"
	"errors"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// CostCalcHandler implements financev1.CostCalcServiceServer by delegating to
// the costcalc application handlers. S8b: single-product TriggerCalcJob is the
// only end-to-end path; other scopes return Unimplemented via ErrScopeNotYetSupported.
type CostCalcHandler struct {
	financev1.UnimplementedCostCalcServiceServer
	triggerH         *costcalc.TriggerJobHandler
	getJobH          *costcalc.GetJobHandler
	listJobsH        *costcalc.ListJobsHandler
	listChunksH      *costcalc.ListChunksHandler
	listJobProductsH *costcalc.ListJobProductsHandler
	cancelJobH       *costcalc.CancelJobHandler
	getResultH       *costcalc.GetCostResultHandler
	getBreakdownH    *costcalc.GetCostBreakdownHandler
	listHistoryH     *costcalc.ListCostHistoryHandler
	listResultsH     *costcalc.ListCostResultsHandler
	verifyH          *costcalc.VerifyCostHandler
	approveH         *costcalc.ApproveCostHandler
	// svc gives ProcessChunkInternal direct access to Service.ProcessChunk;
	// the worker invokes this RPC with the chunk payload off the RMQ queue.
	svc *costcalc.Service
}

// NewCostCalcHandler wires the 11 application handlers + the Service behind the
// gRPC service. The Service is required so the worker-facing
// ProcessChunkInternal RPC can call Service.ProcessChunk directly.
func NewCostCalcHandler(
	svc *costcalc.Service,
	triggerH *costcalc.TriggerJobHandler,
	getJobH *costcalc.GetJobHandler,
	listJobsH *costcalc.ListJobsHandler,
	listChunksH *costcalc.ListChunksHandler,
	listJobProductsH *costcalc.ListJobProductsHandler,
	cancelJobH *costcalc.CancelJobHandler,
	getResultH *costcalc.GetCostResultHandler,
	getBreakdownH *costcalc.GetCostBreakdownHandler,
	listHistoryH *costcalc.ListCostHistoryHandler,
	listResultsH *costcalc.ListCostResultsHandler,
	verifyH *costcalc.VerifyCostHandler,
	approveH *costcalc.ApproveCostHandler,
) *CostCalcHandler {
	return &CostCalcHandler{
		svc:              svc,
		triggerH:         triggerH,
		getJobH:          getJobH,
		listJobsH:        listJobsH,
		listChunksH:      listChunksH,
		listJobProductsH: listJobProductsH,
		cancelJobH:       cancelJobH,
		getResultH:       getResultH,
		getBreakdownH:    getBreakdownH,
		listHistoryH:     listHistoryH,
		listResultsH:     listResultsH,
		verifyH:          verifyH,
		approveH:         approveH,
	}
}

// =============================================================================
// RPCs
// =============================================================================

// TriggerCalcJob creates a calc job and (for SINGLE_PRODUCT) runs it inline.
func (h *CostCalcHandler) TriggerCalcJob(ctx context.Context, req *financev1.TriggerCalcJobRequest) (*financev1.TriggerCalcJobResponse, error) {
	actor, _ := GetUserIDFromCtx(ctx)
	job, err := h.triggerH.Handle(ctx, costcalc.TriggerCommand{
		Period:              req.GetPeriod(),
		CalcType:            protoToCalcType(req.GetCalculationType()),
		Scope:               protoToScope(req.GetScope()),
		ProductSysID:        req.GetProductSysId(),
		RouteHeadID:         req.GetRouteHeadId(),
		ProductTypeIDFilter: req.GetProductTypeIdFilter(),
		TriggeredBy:         "MANUAL",
		Actor:               actor,
	})
	if err != nil {
		return &financev1.TriggerCalcJobResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.TriggerCalcJobResponse{
		Base: successResponse("calc job triggered"),
		Job:  jobToProto(job),
	}, nil
}

// GetCalcJob fetches one job by id.
func (h *CostCalcHandler) GetCalcJob(ctx context.Context, req *financev1.GetCalcJobRequest) (*financev1.GetCalcJobResponse, error) {
	job, err := h.getJobH.Handle(ctx, costcalc.GetJobQuery{JobID: req.GetJobId()})
	if err != nil {
		return &financev1.GetCalcJobResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.GetCalcJobResponse{
		Base: successResponse("OK"),
		Job:  jobToProto(job),
	}, nil
}

// ListCalcJobs returns a paginated, filtered list of jobs.
func (h *CostCalcHandler) ListCalcJobs(ctx context.Context, req *financev1.ListCalcJobsRequest) (*financev1.ListCalcJobsResponse, error) {
	page := int(req.GetPagination().GetPage())
	pageSize := int(req.GetPagination().GetPageSize())
	res, err := h.listJobsH.Handle(ctx, costcalc.ListJobsQuery{
		Period:      req.GetPeriod(),
		CalcType:    protoToCalcType(req.GetCalculationType()),
		Status:      protoToCalcJobStatus(req.GetStatus()),
		TriggeredBy: req.GetTriggeredBy(),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		return &financev1.ListCalcJobsResponse{Base: costCalcErrToBase(err)}, nil
	}
	items := make([]*financev1.CalJob, 0, len(res.Items))
	for _, j := range res.Items {
		items = append(items, jobToProto(j))
	}
	return &financev1.ListCalcJobsResponse{
		Base:       successResponse("OK"),
		Items:      items,
		Pagination: calcPaginationResponse(res.Page, res.PageSize, res.Total),
	}, nil
}

// ListCalcJobChunks returns chunks for one job.
func (h *CostCalcHandler) ListCalcJobChunks(ctx context.Context, req *financev1.ListCalcJobChunksRequest) (*financev1.ListCalcJobChunksResponse, error) {
	q := costcalc.ListChunksQuery{
		JobID:    req.GetJobId(),
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	}
	if w := req.GetWaveNo(); w > 0 {
		wave := int(w)
		q.WaveNo = &wave
	}
	if s := req.GetStatus(); s != financev1.ChunkStatus_CHUNK_STATUS_UNSPECIFIED {
		cs := protoToChunkStatus(s)
		q.Status = &cs
	}
	res, err := h.listChunksH.Handle(ctx, q)
	if err != nil {
		return &financev1.ListCalcJobChunksResponse{Base: costCalcErrToBase(err)}, nil
	}
	items := make([]*financev1.CalJobChunk, 0, len(res.Items))
	for _, c := range res.Items {
		items = append(items, chunkToProto(c))
	}
	return &financev1.ListCalcJobChunksResponse{
		Base:       successResponse("OK"),
		Items:      items,
		Pagination: calcPaginationResponse(res.Page, res.PageSize, res.Total),
	}, nil
}

// ListCalcJobProducts returns per-product execution rows for one job.
func (h *CostCalcHandler) ListCalcJobProducts(ctx context.Context, req *financev1.ListCalcJobProductsRequest) (*financev1.ListCalcJobProductsResponse, error) {
	res, err := h.listJobProductsH.Handle(ctx, costcalc.ListJobProductsQuery{
		JobID:    req.GetJobId(),
		Status:   protoToJobProductStatus(req.GetStatus()),
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	})
	if err != nil {
		return &financev1.ListCalcJobProductsResponse{Base: costCalcErrToBase(err)}, nil
	}
	items := make([]*financev1.CalJobProduct, 0, len(res.Items))
	for _, p := range res.Items {
		items = append(items, jobProductToProto(p))
	}
	return &financev1.ListCalcJobProductsResponse{
		Base:       successResponse("OK"),
		Items:      items,
		Pagination: calcPaginationResponse(res.Page, res.PageSize, res.Total),
	}, nil
}

// CancelCalcJob cancels a running job.
func (h *CostCalcHandler) CancelCalcJob(ctx context.Context, req *financev1.CancelCalcJobRequest) (*financev1.CancelCalcJobResponse, error) {
	actor, _ := GetUserIDFromCtx(ctx)
	job, err := h.cancelJobH.Handle(ctx, costcalc.CancelJobCommand{
		JobID:  req.GetJobId(),
		Actor:  actor,
		Reason: req.GetReason(),
	})
	if err != nil {
		return &financev1.CancelCalcJobResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.CancelCalcJobResponse{
		Base: successResponse("calc job cancelled"),
		Job:  jobToProto(job),
	}, nil
}

// GetCostResult returns the active cost result for a product/period/type.
func (h *CostCalcHandler) GetCostResult(ctx context.Context, req *financev1.GetCostResultRequest) (*financev1.GetCostResultResponse, error) {
	r, err := h.getResultH.Handle(ctx, costcalc.GetCostResultQuery{
		ProductSysID: req.GetProductSysId(),
		Period:       req.GetPeriod(),
		CalcType:     protoToCalcType(req.GetCalculationType()),
	})
	if err != nil {
		return &financev1.GetCostResultResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.GetCostResultResponse{
		Base:   successResponse("OK"),
		Result: resultToProto(r),
	}, nil
}

// GetCostBreakdown returns the full drill-down view.
func (h *CostCalcHandler) GetCostBreakdown(ctx context.Context, req *financev1.GetCostBreakdownRequest) (*financev1.GetCostBreakdownResponse, error) {
	view, err := h.getBreakdownH.Handle(ctx, costcalc.GetCostBreakdownQuery{
		ProductSysID: req.GetProductSysId(),
		Period:       req.GetPeriod(),
		CalcType:     protoToCalcType(req.GetCalculationType()),
	})
	if err != nil {
		return &financev1.GetCostBreakdownResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.GetCostBreakdownResponse{
		Base:      successResponse("OK"),
		Breakdown: breakdownToProto(view),
	}, nil
}

// ListCostHistory returns versioned cost history for a product.
func (h *CostCalcHandler) ListCostHistory(ctx context.Context, req *financev1.ListCostHistoryRequest) (*financev1.ListCostHistoryResponse, error) {
	res, err := h.listHistoryH.Handle(ctx, costcalc.ListCostHistoryQuery{
		ProductSysID: req.GetProductSysId(),
		CalcType:     protoToCalcType(req.GetCalculationType()),
		Page:         int(req.GetPagination().GetPage()),
		PageSize:     int(req.GetPagination().GetPageSize()),
	})
	if err != nil {
		return &financev1.ListCostHistoryResponse{Base: costCalcErrToBase(err)}, nil
	}
	items := make([]*financev1.CostHistoryEntry, 0, len(res.Items))
	for _, r := range res.Items {
		items = append(items, historyEntryToProto(r))
	}
	return &financev1.ListCostHistoryResponse{
		Base:       successResponse("OK"),
		Items:      items,
		Pagination: calcPaginationResponse(res.Page, res.PageSize, res.Total),
	}, nil
}

// ListCostResults lists active cost results across products for a period.
func (h *CostCalcHandler) ListCostResults(ctx context.Context, req *financev1.ListCostResultsRequest) (*financev1.ListCostResultsResponse, error) {
	res, err := h.listResultsH.Handle(ctx, costcalc.ListCostResultsQuery{
		Period:   req.GetPeriod(),
		CalcType: protoToCalcType(req.GetCalculationType()),
		Status:   protoToResultStatusString(req.GetStatus()),
		Search:   req.GetSearch(),
		Page:     int(req.GetPagination().GetPage()),
		PageSize: int(req.GetPagination().GetPageSize()),
	})
	if err != nil {
		return &financev1.ListCostResultsResponse{Base: costCalcErrToBase(err)}, nil
	}
	items := make([]*financev1.CostResult, 0, len(res.Items))
	for _, s := range res.Items {
		items = append(items, summaryToProto(s))
	}
	return &financev1.ListCostResultsResponse{
		Base:           successResponse("OK"),
		Items:          items,
		Pagination:     calcPaginationResponse(res.Page, res.PageSize, res.Total),
		ResolvedPeriod: res.ResolvedPeriod,
	}, nil
}

// VerifyCostResult transitions a CALCULATED result to VERIFIED.
func (h *CostCalcHandler) VerifyCostResult(ctx context.Context, req *financev1.VerifyCostResultRequest) (*financev1.VerifyCostResultResponse, error) {
	actor, _ := GetUserIDFromCtx(ctx)
	if err := h.verifyH.Handle(ctx, costcalc.VerifyCostCommand{
		CostID: req.GetCostId(),
		Actor:  actor,
	}); err != nil {
		return &financev1.VerifyCostResultResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.VerifyCostResultResponse{
		Base: successResponse("cost result verified"),
	}, nil
}

// ApproveCostResult transitions a VERIFIED result to APPROVED.
func (h *CostCalcHandler) ApproveCostResult(ctx context.Context, req *financev1.ApproveCostResultRequest) (*financev1.ApproveCostResultResponse, error) {
	actor, _ := GetUserIDFromCtx(ctx)
	if err := h.approveH.Handle(ctx, costcalc.ApproveCostCommand{
		CostID: req.GetCostId(),
		Actor:  actor,
	}); err != nil {
		return &financev1.ApproveCostResultResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.ApproveCostResultResponse{
		Base: successResponse("cost result approved"),
	}, nil
}

// ProcessChunkInternal computes one chunk of products synchronously.
// Called by finance-cost-worker after consuming a chunk message from RMQ.
// This is a passthrough to Service.ProcessChunk — the chunk row lifecycle and
// per-product persistence are handled inside the application layer.
func (h *CostCalcHandler) ProcessChunkInternal(ctx context.Context, req *financev1.ProcessChunkInternalRequest) (*financev1.ProcessChunkInternalResponse, error) {
	out, err := h.svc.ProcessChunk(ctx, costcalc.ProcessChunkInput{
		JobID:    req.GetJobId(),
		ChunkID:  req.GetChunkId(),
		Period:   req.GetPeriod(),
		CalcType: protoToCalcType(req.GetCalculationType()),
		Products: req.GetProductIds(),
		Actor:    req.GetActor(),
	})
	if err != nil {
		return &financev1.ProcessChunkInternalResponse{Base: costCalcErrToBase(err)}, nil
	}
	return &financev1.ProcessChunkInternalResponse{
		Base:         successResponse("chunk processed"),
		SuccessCount: safeIntToInt32(out.Success),
		FailedCount:  safeIntToInt32(out.Failed),
		BlockedCount: safeIntToInt32(out.Blocked),
	}, nil
}

// =============================================================================
// enum mappers
// =============================================================================

func protoToCalcType(p financev1.CalculationType) costcalcdom.CalculationType {
	switch p {
	case financev1.CalculationType_CALCULATION_TYPE_ACTUAL:
		return costcalcdom.CalcTypeActual
	case financev1.CalculationType_CALCULATION_TYPE_FORECAST:
		return costcalcdom.CalcTypeForecast
	case financev1.CalculationType_CALCULATION_TYPE_SELLING:
		return costcalcdom.CalcTypeSelling
	case financev1.CalculationType_CALCULATION_TYPE_UNSPECIFIED:
		return ""
	}
	return ""
}

func calcTypeToProto(t costcalcdom.CalculationType) financev1.CalculationType {
	switch t {
	case costcalcdom.CalcTypeActual:
		return financev1.CalculationType_CALCULATION_TYPE_ACTUAL
	case costcalcdom.CalcTypeForecast:
		return financev1.CalculationType_CALCULATION_TYPE_FORECAST
	case costcalcdom.CalcTypeSelling:
		return financev1.CalculationType_CALCULATION_TYPE_SELLING
	}
	return financev1.CalculationType_CALCULATION_TYPE_UNSPECIFIED
}

func protoToScope(p financev1.CalcJobScope) costcalcdom.JobScope {
	switch p {
	case financev1.CalcJobScope_CALC_JOB_SCOPE_ALL:
		return costcalcdom.ScopeAll
	case financev1.CalcJobScope_CALC_JOB_SCOPE_FILTERED:
		return costcalcdom.ScopeFiltered
	case financev1.CalcJobScope_CALC_JOB_SCOPE_SINGLE_PRODUCT:
		return costcalcdom.ScopeSingleProduct
	case financev1.CalcJobScope_CALC_JOB_SCOPE_SINGLE_ROUTE:
		return costcalcdom.ScopeSingleRoute
	case financev1.CalcJobScope_CALC_JOB_SCOPE_UNSPECIFIED:
		return ""
	}
	return ""
}

func scopeToProto(s costcalcdom.JobScope) financev1.CalcJobScope {
	switch s {
	case costcalcdom.ScopeAll:
		return financev1.CalcJobScope_CALC_JOB_SCOPE_ALL
	case costcalcdom.ScopeFiltered:
		return financev1.CalcJobScope_CALC_JOB_SCOPE_FILTERED
	case costcalcdom.ScopeSingleProduct:
		return financev1.CalcJobScope_CALC_JOB_SCOPE_SINGLE_PRODUCT
	case costcalcdom.ScopeSingleRoute:
		return financev1.CalcJobScope_CALC_JOB_SCOPE_SINGLE_ROUTE
	}
	return financev1.CalcJobScope_CALC_JOB_SCOPE_UNSPECIFIED
}

func protoToCalcJobStatus(p financev1.CalcJobStatus) costcalcdom.JobStatus {
	switch p {
	case financev1.CalcJobStatus_CALC_JOB_STATUS_QUEUED:
		return costcalcdom.JobStatusQueued
	case financev1.CalcJobStatus_CALC_JOB_STATUS_PLANNING:
		return costcalcdom.JobStatusPlanning
	case financev1.CalcJobStatus_CALC_JOB_STATUS_PROCESSING:
		return costcalcdom.JobStatusProcessing
	case financev1.CalcJobStatus_CALC_JOB_STATUS_SUCCESS:
		return costcalcdom.JobStatusSuccess
	case financev1.CalcJobStatus_CALC_JOB_STATUS_PARTIAL_FAILED:
		return costcalcdom.JobStatusPartialFailed
	case financev1.CalcJobStatus_CALC_JOB_STATUS_FAILED:
		return costcalcdom.JobStatusFailed
	case financev1.CalcJobStatus_CALC_JOB_STATUS_CANCELLED:
		return costcalcdom.JobStatusCancelled
	case financev1.CalcJobStatus_CALC_JOB_STATUS_UNSPECIFIED:
		return ""
	}
	return ""
}

func calcJobStatusToProto(s costcalcdom.JobStatus) financev1.CalcJobStatus {
	switch s {
	case costcalcdom.JobStatusQueued:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_QUEUED
	case costcalcdom.JobStatusPlanning:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_PLANNING
	case costcalcdom.JobStatusProcessing:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_PROCESSING
	case costcalcdom.JobStatusSuccess:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_SUCCESS
	case costcalcdom.JobStatusPartialFailed:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_PARTIAL_FAILED
	case costcalcdom.JobStatusFailed:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_FAILED
	case costcalcdom.JobStatusCancelled:
		return financev1.CalcJobStatus_CALC_JOB_STATUS_CANCELLED
	}
	return financev1.CalcJobStatus_CALC_JOB_STATUS_UNSPECIFIED
}

func protoToChunkStatus(p financev1.ChunkStatus) costcalcdom.ChunkStatus {
	switch p {
	case financev1.ChunkStatus_CHUNK_STATUS_QUEUED:
		return costcalcdom.ChunkStatusQueued
	case financev1.ChunkStatus_CHUNK_STATUS_DISPATCHED:
		return costcalcdom.ChunkStatusDispatched
	case financev1.ChunkStatus_CHUNK_STATUS_PROCESSING:
		return costcalcdom.ChunkStatusProcessing
	case financev1.ChunkStatus_CHUNK_STATUS_SUCCESS:
		return costcalcdom.ChunkStatusSuccess
	case financev1.ChunkStatus_CHUNK_STATUS_PARTIAL_FAILED:
		return costcalcdom.ChunkStatusPartialFailed
	case financev1.ChunkStatus_CHUNK_STATUS_FAILED:
		return costcalcdom.ChunkStatusFailed
	case financev1.ChunkStatus_CHUNK_STATUS_UNSPECIFIED:
		return ""
	}
	return ""
}

func chunkStatusToProto(s costcalcdom.ChunkStatus) financev1.ChunkStatus {
	switch s {
	case costcalcdom.ChunkStatusQueued:
		return financev1.ChunkStatus_CHUNK_STATUS_QUEUED
	case costcalcdom.ChunkStatusDispatched:
		return financev1.ChunkStatus_CHUNK_STATUS_DISPATCHED
	case costcalcdom.ChunkStatusProcessing:
		return financev1.ChunkStatus_CHUNK_STATUS_PROCESSING
	case costcalcdom.ChunkStatusSuccess:
		return financev1.ChunkStatus_CHUNK_STATUS_SUCCESS
	case costcalcdom.ChunkStatusPartialFailed:
		return financev1.ChunkStatus_CHUNK_STATUS_PARTIAL_FAILED
	case costcalcdom.ChunkStatusFailed:
		return financev1.ChunkStatus_CHUNK_STATUS_FAILED
	}
	return financev1.ChunkStatus_CHUNK_STATUS_UNSPECIFIED
}

func protoToJobProductStatus(p financev1.JobProductStatus) costcalcdom.JobProductStatus {
	switch p {
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_PENDING:
		return costcalcdom.JobProductStatusPending
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_READY:
		return costcalcdom.JobProductStatusReady
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_CALCULATING:
		return costcalcdom.JobProductStatusCalculating
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_SUCCESS:
		return costcalcdom.JobProductStatusSuccess
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_FAILED:
		return costcalcdom.JobProductStatusFailed
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_BLOCKED:
		return costcalcdom.JobProductStatusBlocked
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_SKIPPED:
		return costcalcdom.JobProductStatusSkipped
	case financev1.JobProductStatus_JOB_PRODUCT_STATUS_UNSPECIFIED:
		return ""
	}
	return ""
}

func jobProductStatusToProto(s costcalcdom.JobProductStatus) financev1.JobProductStatus {
	switch s {
	case costcalcdom.JobProductStatusPending:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_PENDING
	case costcalcdom.JobProductStatusReady:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_READY
	case costcalcdom.JobProductStatusCalculating:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_CALCULATING
	case costcalcdom.JobProductStatusSuccess:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_SUCCESS
	case costcalcdom.JobProductStatusFailed:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_FAILED
	case costcalcdom.JobProductStatusBlocked:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_BLOCKED
	case costcalcdom.JobProductStatusSkipped:
		return financev1.JobProductStatus_JOB_PRODUCT_STATUS_SKIPPED
	}
	return financev1.JobProductStatus_JOB_PRODUCT_STATUS_UNSPECIFIED
}

func resultStatusToProto(s costcalcdom.ResultStatus) financev1.CostResultStatus {
	switch s {
	case costcalcdom.ResultStatusCalculated:
		return financev1.CostResultStatus_COST_RESULT_STATUS_CALCULATED
	case costcalcdom.ResultStatusVerified:
		return financev1.CostResultStatus_COST_RESULT_STATUS_VERIFIED
	case costcalcdom.ResultStatusApproved:
		return financev1.CostResultStatus_COST_RESULT_STATUS_APPROVED
	case costcalcdom.ResultStatusSuperseded:
		return financev1.CostResultStatus_COST_RESULT_STATUS_SUPERSEDED
	}
	return financev1.CostResultStatus_COST_RESULT_STATUS_UNSPECIFIED
}

// =============================================================================
// aggregate -> proto mappers
// =============================================================================

func jobToProto(j *costcalcdom.Job) *financev1.CalJob {
	if j == nil {
		return nil
	}
	return &financev1.CalJob{
		JobId:             j.ID(),
		JobCode:           j.Code(),
		Period:            j.Period(),
		CalculationType:   calcTypeToProto(j.CalcType()),
		Scope:             scopeToProto(j.Scope()),
		ProductFilterJson: string(j.ProductFilter()),
		Status:            calcJobStatusToProto(j.Status()),
		Priority:          safeIntToInt32(j.Priority()),
		TotalProducts:     safeIntToInt32(j.TotalProducts()),
		TotalChunks:       safeIntToInt32(j.TotalChunks()),
		TotalWaves:        safeIntToInt32(j.TotalWaves()),
		ProcessedChunks:   safeIntToInt32(j.ProcessedChunks()),
		SuccessCount:      safeIntToInt32(j.SuccessCount()),
		FailedCount:       safeIntToInt32(j.FailedCount()),
		BlockedCount:      safeIntToInt32(j.BlockedCount()),
		ErrorSummaryJson:  string(j.ErrorSummary()),
		TriggeredBy:       j.TriggeredBy(),
		QueuedAt:          timeToProto(j.QueuedAt()),
		StartedAt:         timePtrToProto(j.StartedAt()),
		CompletedAt:       timePtrToProto(j.CompletedAt()),
		DurationMs:        j.DurationMs(),
		CreatedBy:         j.CreatedBy(),
	}
}

func chunkToProto(c *costcalcdom.Chunk) *financev1.CalJobChunk {
	if c == nil {
		return nil
	}
	return &financev1.CalJobChunk{
		ChunkId:      c.ID(),
		JobId:        c.JobID(),
		ChunkNumber:  safeIntToInt32(c.ChunkNumber()),
		WaveNo:       safeIntToInt32(c.WaveNo()),
		ProductIds:   c.ProductIDs(),
		ProductCount: safeIntToInt32(c.ProductCount()),
		Status:       chunkStatusToProto(c.Status()),
		WorkerId:     c.WorkerID(),
		QueuedAt:     timeToProto(c.QueuedAt()),
		DispatchedAt: timePtrToProto(c.DispatchedAt()),
		StartedAt:    timePtrToProto(c.StartedAt()),
		CompletedAt:  timePtrToProto(c.CompletedAt()),
		DurationMs:   safeIntToInt32(c.DurationMs()),
		SuccessCount: safeIntToInt32(c.SuccessCount()),
		FailedCount:  safeIntToInt32(c.FailedCount()),
		ErrorMessage: c.ErrorMessage(),
		RetryCount:   safeIntToInt32(c.RetryCount()),
		MaxRetries:   safeIntToInt32(c.MaxRetries()),
	}
}

func jobProductToProto(p *costcalcdom.JobProduct) *financev1.CalJobProduct {
	if p == nil {
		return nil
	}
	return &financev1.CalJobProduct{
		JobProductId:       p.ID(),
		JobId:              p.JobID(),
		ChunkId:            p.ChunkID(),
		ProductSysId:       p.ProductSysID(),
		RouteHeadId:        p.RouteHeadID(),
		WaveNo:             safeIntToInt32(p.WaveNo()),
		Status:             jobProductStatusToProto(p.Status()),
		BlockReason:        p.BlockReason(),
		StartedAt:          timePtrToProto(p.StartedAt()),
		CompletedAt:        timePtrToProto(p.CompletedAt()),
		DurationMs:         safeIntToInt32(p.DurationMs()),
		CostId:             p.CostID(),
		ErrorMessage:       p.ErrorMessage(),
		CalculationLogJson: string(p.CalculationLog()),
	}
}

func resultToProto(r *costcalcdom.Result) *financev1.CostResult {
	if r == nil {
		return nil
	}
	return &financev1.CostResult{
		CostId:          r.ID(),
		ProductSysId:    r.ProductSysID(),
		Period:          r.Period(),
		CalculationType: calcTypeToProto(r.CalcType()),
		RouteHeadId:     r.RouteHeadID(),
		Version:         safeIntToInt32(r.Version()),
		CostPerUnit:     formatNumeric(r.CostPerUnit()),
		TotalRmCost:     formatNumeric(r.TotalRMCost()),
		TotalConversion: formatNumeric(r.TotalConv()),
		TotalCost:       formatNumeric(r.TotalCost()),
		UomId:           safeIntToInt32(r.UomID()),
		CurrencyCode:    r.Currency(),
		Status:          resultStatusToProto(r.Status()),
		JobId:           r.JobID(),
		CalculatedAt:    timeToProto(r.CalculatedAt()),
		CalculatedBy:    r.CalculatedBy(),
		VerifiedAt:      timePtrToProto(r.VerifiedAt()),
		VerifiedBy:      r.VerifiedBy(),
	}
}

// summaryToProto maps a flat ResultSummary (with resolved product code/name)
// to the CostResult proto for the cross-product list view.
func summaryToProto(s *costcalcdom.ResultSummary) *financev1.CostResult {
	if s == nil {
		return nil
	}
	return &financev1.CostResult{
		CostId:          s.CostID,
		ProductSysId:    s.ProductSysID,
		ProductCode:     s.ProductCode,
		ProductName:     s.ProductName,
		Period:          s.Period,
		CalculationType: calcTypeToProto(s.CalcType),
		RouteHeadId:     s.RouteHeadID,
		Version:         safeIntToInt32(s.Version),
		CostPerUnit:     formatNumeric(s.CostPerUnit),
		TotalRmCost:     formatNumeric(s.TotalRMCost),
		TotalConversion: formatNumeric(s.TotalConv),
		TotalCost:       formatNumeric(s.TotalCost),
		UomId:           safeIntToInt32(s.UOMID),
		CurrencyCode:    s.CurrencyCode,
		Status:          resultStatusToProto(costcalcdom.ResultStatus(s.Status)),
		JobId:           s.JobID,
		CalculatedAt:    timeToProto(s.CalculatedAt),
		CalculatedBy:    s.CalculatedBy,
	}
}

// protoToResultStatusString maps the filter enum to the DB status string
// ("" when UNSPECIFIED — the repo then returns active rows only).
func protoToResultStatusString(p financev1.CostResultStatus) string {
	switch p {
	case financev1.CostResultStatus_COST_RESULT_STATUS_CALCULATED:
		return paramCategoryCalculated
	case financev1.CostResultStatus_COST_RESULT_STATUS_VERIFIED:
		return "VERIFIED"
	case financev1.CostResultStatus_COST_RESULT_STATUS_APPROVED:
		return "APPROVED"
	case financev1.CostResultStatus_COST_RESULT_STATUS_SUPERSEDED:
		return "SUPERSEDED"
	default:
		return ""
	}
}

func breakdownToProto(view *costcalc.CostBreakdownView) *financev1.CostBreakdown {
	if view == nil {
		return nil
	}
	byLevel := make([]*financev1.LevelBreakdown, 0, len(view.CostByLevel))
	for _, lc := range view.CostByLevel {
		byLevel = append(byLevel, &financev1.LevelBreakdown{
			Level:            lc.Level,
			CostContribution: formatNumeric(lc.RMCost + lc.Conversion),
		})
	}
	rmDetails := make([]*financev1.CostRMDetail, 0, len(view.RMCostDetail))
	for _, d := range view.RMCostDetail {
		rmDetails = append(rmDetails, &financev1.CostRMDetail{
			RmType:       d.RMType,
			RefCode:      d.RefCode,
			ShadeCode:    d.ShadeCode,
			UnitCost:     formatNumeric(d.UnitCost),
			Ratio:        formatNumeric(d.Ratio),
			Contribution: formatNumeric(d.Contribution),
		})
	}
	formulaTrace := make([]*financev1.FormulaEval, 0, len(view.FormulaTrace))
	for _, ft := range view.FormulaTrace {
		inputs := make(map[string]string, len(ft.Inputs))
		for k, v := range ft.Inputs {
			inputs[k] = formatNumeric(v)
		}
		formulaTrace = append(formulaTrace, &financev1.FormulaEval{
			FormulaCode:     ft.FormulaCode,
			Expression:      ft.Expression,
			Inputs:          inputs,
			OutputParamCode: ft.ResultParamCode,
			OutputValue:     formatNumeric(ft.Output),
		})
	}
	paramSnapshot := make(map[string]string, len(view.ParamSnapshot))
	for k, v := range view.ParamSnapshot {
		paramSnapshot[k] = formatNumeric(v)
	}
	return &financev1.CostBreakdown{
		Summary:       resultToProto(view.Result),
		ByLevel:       byLevel,
		RmDetails:     rmDetails,
		FormulaTrace:  formulaTrace,
		ParamSnapshot: paramSnapshot,
	}
}

func historyEntryToProto(r *costcalcdom.Result) *financev1.CostHistoryEntry {
	if r == nil {
		return nil
	}
	return &financev1.CostHistoryEntry{
		CostId:          r.ID(),
		Period:          r.Period(),
		CalculationType: calcTypeToProto(r.CalcType()),
		Version:         safeIntToInt32(r.Version()),
		CostPerUnit:     formatNumeric(r.CostPerUnit()),
		Status:          resultStatusToProto(r.Status()),
		JobId:           r.JobID(),
		CalculatedAt:    timeToProto(r.CalculatedAt()),
		CalculatedBy:    r.CalculatedBy(),
	}
}

// =============================================================================
// small helpers
// =============================================================================

func formatNumeric(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

func timeToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func timePtrToProto(t *time.Time) *timestamppb.Timestamp {
	if t == nil || t.IsZero() {
		return nil
	}
	return timestamppb.New(*t)
}

func calcPaginationResponse(page, pageSize, total int) *commonv1.PaginationResponse {
	if pageSize <= 0 {
		pageSize = 1
	}
	totalPages := safeIntToInt32((total + pageSize - 1) / pageSize)
	return &commonv1.PaginationResponse{
		CurrentPage: safeIntToInt32(page),
		PageSize:    safeIntToInt32(pageSize),
		TotalItems:  int64(total),
		TotalPages:  totalPages,
	}
}

// =============================================================================
// error mapping
// =============================================================================

// costCalcErrToBase maps domain + application errors to a BaseResponse envelope.
func costCalcErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, costcalcdom.ErrJobNotFound):
		return ErrorResponse("404", "calc job not found")
	case errors.Is(err, costcalcdom.ErrCostNotFound):
		return ErrorResponse("404", "cost result not found")
	case errors.Is(err, costcalcdom.ErrJobInvalidStatus):
		return ErrorResponse("409", err.Error())
	case errors.Is(err, costcalcdom.ErrCostInvalidStatus):
		return ErrorResponse("409", err.Error())
	case errors.Is(err, costcalcdom.ErrJobAlreadyRunning),
		errors.Is(err, costcalcdom.ErrCostAlreadyInFlight):
		return ErrorResponse("409", err.Error())
	case errors.Is(err, costcalc.ErrScopeNotYetSupported):
		return ErrorResponse("501", err.Error())
	case errors.Is(err, costcalc.ErrProductRequired),
		errors.Is(err, costcalcdom.ErrInvalidPeriod):
		return ErrorResponse("400", err.Error())
	}
	if s, ok := status.FromError(err); ok && s != nil && s.Code() != codes.Unknown {
		return ErrorResponse(grpcCodeToStatusCode(s.Code()), s.Message())
	}
	// Validation errors from errors_validation.go are plain errors.New(...);
	// surface their text as 400 for client-visible feedback.
	if isCalcValidationErr(err) {
		return ErrorResponse("400", err.Error())
	}
	return ErrorResponse("500", err.Error())
}

// isCalcValidationErr matches the well-known validation strings from
// costcalc/errors_validation.go. Plain errors.New values can't be matched
// with errors.Is, so we string-match by the messages defined there.
func isCalcValidationErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, v := range []string{
		"job_id must be > 0",
		"product_sys_id must be > 0",
		"cost_id must be > 0",
		"period must be YYYYMM",
		"actor required",
	} {
		if msg == v {
			return true
		}
	}
	return false
}
