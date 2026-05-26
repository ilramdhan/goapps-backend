package grpc

import (
	"context"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	jobapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/job"
)

// BIJobHandler implements financev1.BiJobServiceServer.
type BIJobHandler struct {
	financev1.UnimplementedBiJobServiceServer
	listHandler      *jobapp.ListHandler
	listLogsHandler  *jobapp.ListLogsHandler
	triggerHandler   *jobapp.TriggerHandler
	validationHelper *ValidationHelper
}

// NewBIJobHandler constructs the gRPC handler.
func NewBIJobHandler(list *jobapp.ListHandler, listLogs *jobapp.ListLogsHandler, trigger *jobapp.TriggerHandler) (*BIJobHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIJobHandler{
		listHandler:      list,
		listLogsHandler:  listLogs,
		triggerHandler:   trigger,
		validationHelper: v,
	}, nil
}

// ListJobs returns the ETL job registry.
func (h *BIJobHandler) ListJobs(ctx context.Context, req *financev1.ListJobsRequest) (*financev1.ListJobsResponse, error) {
	out, err := h.listHandler.Handle(ctx, req.GetIncludeInactive())
	if err != nil {
		return &financev1.ListJobsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.BiJob, 0, len(out))
	for _, j := range out {
		items = append(items, jobToProto(j))
	}
	return &financev1.ListJobsResponse{
		Base: successResponse("Jobs listed"),
		Data: items,
	}, nil
}

// ListJobLogs returns paginated logs for one job.
func (h *BIJobHandler) ListJobLogs(ctx context.Context, req *financev1.ListJobLogsRequest) (*financev1.ListJobLogsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.ListJobLogsResponse{Base: baseResp}, nil
	}
	result, err := h.listLogsHandler.Handle(ctx, jobapp.ListLogsQuery{
		JobID:    uuidFromString(req.GetJobId()),
		Page:     int(req.GetPage()),
		PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return &financev1.ListJobLogsResponse{Base: biDomainErrorToBase(err)}, nil
	}
	items := make([]*financev1.BiJobLog, 0, len(result.Items))
	for _, l := range result.Items {
		items = append(items, jobLogToProto(l))
	}
	return &financev1.ListJobLogsResponse{
		Base:       successResponse("Job logs listed"),
		Data:       items,
		Pagination: paginationResponse(int(req.GetPage()), int(req.GetPageSize()), result.Total),
	}, nil
}

// TriggerJob fires a manual job run (MVP: placeholder that records RUNNING→SUCCESS).
func (h *BIJobHandler) TriggerJob(ctx context.Context, req *financev1.TriggerJobRequest) (*financev1.TriggerJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.TriggerJobResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	log, err := h.triggerHandler.Handle(ctx, jobapp.TriggerCommand{
		JobID:       uuidFromString(req.GetJobId()),
		TriggeredBy: userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.TriggerJobResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.TriggerJobResponse{
		Base: successResponse("Job triggered"),
		Data: jobLogToProto(log),
	}, nil
}

// Compile-time interface check.
var _ financev1.BiJobServiceServer = (*BIJobHandler)(nil)
