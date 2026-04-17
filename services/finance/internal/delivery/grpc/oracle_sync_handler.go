package grpc

import (
	"context"
	"math"
	"strings"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/oraclesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// OracleSyncHandler implements the OracleSyncServiceServer interface.
type OracleSyncHandler struct {
	financev1.UnimplementedOracleSyncServiceServer
	triggerHandler     *oraclesync.TriggerHandler
	getJobHandler      *oraclesync.GetJobHandler
	listJobsHandler    *oraclesync.ListJobsHandler
	cancelJobHandler   *oraclesync.CancelJobHandler
	listDataHandler    *oraclesync.ListDataHandler
	listPeriodsHandler *oraclesync.ListPeriodsHandler
	validationHelper   *ValidationHelper
}

// NewOracleSyncHandler creates a new OracleSyncHandler.
func NewOracleSyncHandler(
	triggerHandler *oraclesync.TriggerHandler,
	getJobHandler *oraclesync.GetJobHandler,
	listJobsHandler *oraclesync.ListJobsHandler,
	cancelJobHandler *oraclesync.CancelJobHandler,
	listDataHandler *oraclesync.ListDataHandler,
	listPeriodsHandler *oraclesync.ListPeriodsHandler,
) (*OracleSyncHandler, error) {
	validationHelper, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}

	return &OracleSyncHandler{
		triggerHandler:     triggerHandler,
		getJobHandler:      getJobHandler,
		listJobsHandler:    listJobsHandler,
		cancelJobHandler:   cancelJobHandler,
		listDataHandler:    listDataHandler,
		listPeriodsHandler: listPeriodsHandler,
		validationHelper:   validationHelper,
	}, nil
}

// TriggerSync initiates a manual Oracle sync job.
func (h *OracleSyncHandler) TriggerSync(ctx context.Context, req *financev1.TriggerSyncRequest) (*financev1.TriggerSyncResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.TriggerSyncResponse{Base: baseResp}, nil
	}

	cmd := oraclesync.TriggerCommand{
		Period:    req.Period,
		CreatedBy: getUserFromContext(ctx),
	}

	result, err := h.triggerHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.TriggerSyncResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	return &financev1.TriggerSyncResponse{
		Base: successResponse("Sync job created and queued"),
		Data: executionToProto(result.Execution),
	}, nil
}

// GetSyncJob retrieves a specific sync job by ID with execution logs.
func (h *OracleSyncHandler) GetSyncJob(ctx context.Context, req *financev1.GetSyncJobRequest) (*financev1.GetSyncJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.GetSyncJobResponse{Base: baseResp}, nil
	}

	query := oraclesync.GetJobQuery{JobID: req.JobId}
	exec, err := h.getJobHandler.Handle(ctx, query)
	if err != nil {
		return &financev1.GetSyncJobResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	return &financev1.GetSyncJobResponse{
		Base: successResponse("Sync job retrieved"),
		Data: executionToProto(exec),
	}, nil
}

// ListSyncJobs retrieves a paginated list of sync job executions.
func (h *OracleSyncHandler) ListSyncJobs(ctx context.Context, req *financev1.ListSyncJobsRequest) (*financev1.ListSyncJobsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListSyncJobsResponse{Base: baseResp}, nil
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := oraclesync.ListJobsQuery{
		Page:     page,
		PageSize: pageSize,
		JobType:  req.JobType,
		Status:   jobStatusToString(req.Status),
		Period:   req.Period,
		Search:   req.Search,
	}

	result, err := h.listJobsHandler.Handle(ctx, query)
	if err != nil {
		return &financev1.ListSyncJobsResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.SyncJob, len(result.Executions))
	for i, exec := range result.Executions {
		items[i] = executionToProto(exec)
	}

	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32(math.Ceil(float64(result.Total) / float64(pageSize))) //nolint:gosec // bounded by pagination
	}

	return &financev1.ListSyncJobsResponse{
		Base: successResponse("Sync jobs retrieved"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: int32(page),     //nolint:gosec // bounded by validation (1-100)
			PageSize:    int32(pageSize), //nolint:gosec // bounded by validation (1-100)
			TotalItems:  result.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// CancelSyncJob cancels a queued or in-progress sync job.
func (h *OracleSyncHandler) CancelSyncJob(ctx context.Context, req *financev1.CancelSyncJobRequest) (*financev1.CancelSyncJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CancelSyncJobResponse{Base: baseResp}, nil
	}

	cmd := oraclesync.CancelJobCommand{
		JobID:       req.JobId,
		CancelledBy: getUserFromContext(ctx),
	}

	exec, err := h.cancelJobHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.CancelSyncJobResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	return &financev1.CancelSyncJobResponse{
		Base: successResponse("Sync job cancelled"),
		Data: executionToProto(exec),
	}, nil
}

// ListItemConsStockPO retrieves synced item consumption, stock, and PO data.
func (h *OracleSyncHandler) ListItemConsStockPO(ctx context.Context, req *financev1.ListItemConsStockPORequest) (*financev1.ListItemConsStockPOResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListItemConsStockPOResponse{Base: baseResp}, nil
	}

	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := oraclesync.ListDataQuery{
		Page:     page,
		PageSize: pageSize,
		Period:   req.Period,
		ItemCode: req.ItemCode,
		Search:   req.Search,
	}

	result, err := h.listDataHandler.Handle(ctx, query)
	if err != nil {
		return &financev1.ListItemConsStockPOResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.ItemConsStockPO, len(result.Items))
	for i, item := range result.Items {
		items[i] = itemToProto(item)
	}

	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = int32(math.Ceil(float64(result.Total) / float64(pageSize))) //nolint:gosec // bounded by pagination
	}

	return &financev1.ListItemConsStockPOResponse{
		Base: successResponse("Synced data retrieved"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: int32(page),     //nolint:gosec // bounded by validation (1-100)
			PageSize:    int32(pageSize), //nolint:gosec // bounded by validation (1-100)
			TotalItems:  result.Total,
			TotalPages:  totalPages,
		},
	}, nil
}

// ListSyncPeriods retrieves all available sync periods.
func (h *OracleSyncHandler) ListSyncPeriods(ctx context.Context, req *financev1.ListSyncPeriodsRequest) (*financev1.ListSyncPeriodsResponse, error) {
	periods, err := h.listPeriodsHandler.Handle(ctx)
	if err != nil {
		return &financev1.ListSyncPeriodsResponse{Base: syncErrorToBaseResponse(err)}, nil
	}

	return &financev1.ListSyncPeriodsResponse{
		Base:    successResponse("Sync periods retrieved"),
		Periods: periods,
	}, nil
}

// =============================================================================
// Mapping helpers
// =============================================================================

func syncErrorToBaseResponse(err error) *commonv1.BaseResponse {
	if err == nil {
		return successResponse("")
	}

	errMsg := err.Error()

	switch {
	case strings.Contains(errMsg, "not found"):
		return NotFoundResponse(errMsg)
	case strings.Contains(errMsg, "duplicate active job"):
		return ConflictResponse("A sync job for this period is already queued or processing")
	case strings.Contains(errMsg, "already cancelled"):
		return ConflictResponse(errMsg)
	case strings.Contains(errMsg, "not cancellable"):
		return ErrorResponse("400", errMsg)
	case strings.Contains(errMsg, "invalid"):
		return ErrorResponse("400", errMsg)
	default:
		return InternalErrorResponse(errMsg)
	}
}

func executionToProto(exec *job.Execution) *financev1.SyncJob {
	if exec == nil {
		return nil
	}

	proto := &financev1.SyncJob{
		JobId:         exec.ID().String(),
		JobCode:       exec.Code().String(),
		JobType:       exec.JobType().String(),
		JobSubtype:    exec.Subtype(),
		Period:        exec.Period(),
		Status:        stringToJobStatus(exec.Status().String()),
		Priority:      int32(exec.Priority()), //nolint:gosec // priority is 1-10
		Progress:      int32(exec.Progress()), //nolint:gosec // progress is 0-100
		ErrorMessage:  exec.ErrorMessage(),
		ResultSummary: string(exec.ResultSummary()),
		RetryCount:    int32(exec.RetryCount()), //nolint:gosec // small bounded value
		MaxRetries:    int32(exec.MaxRetries()), //nolint:gosec // small bounded value
		QueuedAt:      formatTime(exec.QueuedAt()),
		StartedAt:     formatTimePtr(exec.StartedAt()),
		CompletedAt:   formatTimePtr(exec.CompletedAt()),
		CreatedBy:     exec.CreatedBy(),
		CancelledBy:   exec.CancelledBy(),
		CancelledAt:   formatTimePtr(exec.CancelledAt()),
	}

	for _, logEntry := range exec.Logs() {
		proto.Logs = append(proto.Logs, logToProto(logEntry))
	}

	return proto
}

func logToProto(logEntry *job.ExecutionLog) *financev1.SyncJobLog {
	if logEntry == nil {
		return nil
	}

	proto := &financev1.SyncJobLog{
		LogId:       logEntry.ID().String(),
		JobId:       logEntry.JobID().String(),
		Step:        logEntry.Step(),
		Status:      stringToLogStatus(logEntry.Status().String()),
		Message:     logEntry.Message(),
		Metadata:    string(logEntry.Metadata()),
		StartedAt:   formatTime(logEntry.StartedAt()),
		CompletedAt: formatTimePtr(logEntry.CompletedAt()),
	}

	if logEntry.DurationMs() != nil {
		proto.DurationMs = int32(*logEntry.DurationMs()) //nolint:gosec // duration in ms is bounded
	}

	return proto
}

func itemToProto(item *syncdata.ItemConsStockPO) *financev1.ItemConsStockPO {
	if item == nil {
		return nil
	}

	proto := &financev1.ItemConsStockPO{
		Period:      item.Period,
		ItemCode:    item.ItemCode,
		GradeCode:   item.GradeCode,
		GradeName:   item.GradeName,
		ItemName:    item.ItemName,
		Uom:         item.UOM,
		ConsQty:     derefFloat(item.ConsQty),
		ConsVal:     derefFloat(item.ConsVal),
		ConsRate:    derefFloat(item.ConsRate),
		StoresQty:   derefFloat(item.StoresQty),
		StoresVal:   derefFloat(item.StoresVal),
		StoresRate:  derefFloat(item.StoresRate),
		DeptQty:     derefFloat(item.DeptQty),
		DeptVal:     derefFloat(item.DeptVal),
		DeptRate:    derefFloat(item.DeptRate),
		LastPoQty1:  derefFloat(item.LastPOQty1),
		LastPoVal1:  derefFloat(item.LastPOVal1),
		LastPoRate1: derefFloat(item.LastPORate1),
		LastPoDt1:   formatTimePtr(item.LastPODt1),
		LastPoQty2:  derefFloat(item.LastPOQty2),
		LastPoVal2:  derefFloat(item.LastPOVal2),
		LastPoRate2: derefFloat(item.LastPORate2),
		LastPoDt2:   formatTimePtr(item.LastPODt2),
		LastPoQty3:  derefFloat(item.LastPOQty3),
		LastPoVal3:  derefFloat(item.LastPOVal3),
		LastPoRate3: derefFloat(item.LastPORate3),
		LastPoDt3:   formatTimePtr(item.LastPODt3),
	}

	if item.SyncedAt != nil {
		proto.SyncedAt = item.SyncedAt.Format(time.RFC3339)
	}
	if item.SyncedByJob != nil {
		proto.SyncedByJob = item.SyncedByJob.String()
	}

	return proto
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func stringToJobStatus(s string) financev1.JobStatus {
	switch s {
	case "QUEUED":
		return financev1.JobStatus_JOB_STATUS_QUEUED
	case "PROCESSING":
		return financev1.JobStatus_JOB_STATUS_PROCESSING
	case "SUCCESS":
		return financev1.JobStatus_JOB_STATUS_SUCCESS
	case "FAILED":
		return financev1.JobStatus_JOB_STATUS_FAILED
	case "CANCELLED":
		return financev1.JobStatus_JOB_STATUS_CANCELLED
	default:
		return financev1.JobStatus_JOB_STATUS_UNSPECIFIED
	}
}

func jobStatusToString(s financev1.JobStatus) string {
	switch s {
	case financev1.JobStatus_JOB_STATUS_QUEUED:
		return "QUEUED"
	case financev1.JobStatus_JOB_STATUS_PROCESSING:
		return "PROCESSING"
	case financev1.JobStatus_JOB_STATUS_SUCCESS:
		return "SUCCESS"
	case financev1.JobStatus_JOB_STATUS_FAILED:
		return "FAILED"
	case financev1.JobStatus_JOB_STATUS_CANCELLED:
		return "CANCELLED"
	case financev1.JobStatus_JOB_STATUS_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func stringToLogStatus(s string) financev1.JobLogStatus {
	switch s {
	case "STARTED":
		return financev1.JobLogStatus_JOB_LOG_STATUS_STARTED
	case "SUCCESS":
		return financev1.JobLogStatus_JOB_LOG_STATUS_SUCCESS
	case "FAILED":
		return financev1.JobLogStatus_JOB_LOG_STATUS_FAILED
	case "SKIPPED":
		return financev1.JobLogStatus_JOB_LOG_STATUS_SKIPPED
	default:
		return financev1.JobLogStatus_JOB_LOG_STATUS_UNSPECIFIED
	}
}
