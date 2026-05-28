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
	createHandler    *jobapp.CreateHandler
	updateHandler    *jobapp.UpdateHandler
	deleteHandler    *jobapp.DeleteHandler
	validationHelper *ValidationHelper
}

// NewBIJobHandler constructs the gRPC handler.
func NewBIJobHandler(
	list *jobapp.ListHandler,
	listLogs *jobapp.ListLogsHandler,
	trigger *jobapp.TriggerHandler,
	create *jobapp.CreateHandler,
	update *jobapp.UpdateHandler,
	del *jobapp.DeleteHandler,
) (*BIJobHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &BIJobHandler{
		listHandler:      list,
		listLogsHandler:  listLogs,
		triggerHandler:   trigger,
		createHandler:    create,
		updateHandler:    update,
		deleteHandler:    del,
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
		items = append(items, biJobToProto(j))
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
		Pagination: biPaginationResponse(int(req.GetPage()), int(req.GetPageSize()), result.Total),
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

// CreateBiJob registers a new ETL job.
func (h *BIJobHandler) CreateBiJob(ctx context.Context, req *financev1.CreateBiJobRequest) (*financev1.CreateBiJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.CreateBiJobResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	// Resolve source_code → source_id via the data source repository lookup embedded in the
	// create command; the delivery layer passes source_code and the repo resolves it.
	// For now the handler passes source_code through; the create_handler stores it as SourceCode,
	// and the postgres layer uses a sub-select to resolve the FK.
	out, err := h.createHandler.Handle(ctx, jobapp.CreateCommand{
		JobName:         req.GetJobName(),
		SourceCode:      req.GetSourceCode(),
		TargetType:      req.GetTargetType(),
		ScheduleCron:    req.GetScheduleCron(),
		OracleProcedure: req.GetOracleProcedure(),
		Config:          structToMap(req.GetConfig()),
		IsActive:        req.GetIsActive(),
		CreatedBy:       userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.CreateBiJobResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.CreateBiJobResponse{
		Base: successResponse("Job created"),
		Data: biJobToProto(out),
	}, nil
}

// UpdateBiJob mutates an existing ETL job.
func (h *BIJobHandler) UpdateBiJob(ctx context.Context, req *financev1.UpdateBiJobRequest) (*financev1.UpdateBiJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.UpdateBiJobResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	cmd := jobapp.UpdateCommand{
		JobID:     uuidFromString(req.GetJobId()),
		UpdatedBy: userUUIDFromContext(userID),
	}
	if req.ScheduleCron != nil {
		v := req.GetScheduleCron()
		cmd.ScheduleCron = &v
	}
	if req.OracleProcedure != nil {
		v := req.GetOracleProcedure()
		cmd.OracleProcedure = &v
	}
	if req.Config != nil {
		cmd.Config = structToMap(req.GetConfig())
	}
	if req.IsActive != nil {
		v := req.GetIsActive()
		cmd.IsActive = &v
	}
	out, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		return &financev1.UpdateBiJobResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.UpdateBiJobResponse{
		Base: successResponse("Job updated"),
		Data: biJobToProto(out),
	}, nil
}

// DeleteBiJob soft-disables a job (sets is_active=false, preserves logs).
func (h *BIJobHandler) DeleteBiJob(ctx context.Context, req *financev1.DeleteBiJobRequest) (*financev1.DeleteBiJobResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		return &financev1.DeleteBiJobResponse{Base: baseResp}, nil
	}
	userID, _ := GetUserIDFromCtx(ctx)
	err := h.deleteHandler.Handle(ctx, jobapp.DeleteCommand{
		JobID:     uuidFromString(req.GetJobId()),
		DeletedBy: userUUIDFromContext(userID),
	})
	if err != nil {
		return &financev1.DeleteBiJobResponse{Base: biDomainErrorToBase(err)}, nil
	}
	return &financev1.DeleteBiJobResponse{
		Base: successResponse("Job disabled"),
	}, nil
}

// Compile-time interface check.
var _ financev1.BiJobServiceServer = (*BIJobHandler)(nil)
