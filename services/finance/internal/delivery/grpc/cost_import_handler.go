// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"bytes"
	"context"
	"fmt"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cappapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductapplicableparam"
	cpmapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	cppapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/rabbitmq"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

const importPresignValidity = 15 * time.Minute

// CostDataImportHandler implements financev1.CostDataImportServiceServer.
type CostDataImportHandler struct {
	financev1.UnimplementedCostDataImportServiceServer
	jobRepo         costimportjob.Repository
	storage         storage.Service
	cappExport      *cappapp.ExportHandler
	cappTemplate    *cappapp.TemplateHandler
	cppExport       *cppapp.ExportHandler
	cppTemplate     *cppapp.TemplateHandler
	cpmExport       *cpmapp.ExportHandler
	cpmTemplate     *cpmapp.TemplateHandler
	importPublisher *rabbitmq.JobPublisherAdapter
}

// NewCostDataImportHandler constructs the handler.
func NewCostDataImportHandler(
	jobRepo costimportjob.Repository,
	storageSvc storage.Service,
	cappExport *cappapp.ExportHandler,
	cappTemplate *cappapp.TemplateHandler,
	cppExport *cppapp.ExportHandler,
	cppTemplate *cppapp.TemplateHandler,
	cpmExport *cpmapp.ExportHandler,
	cpmTemplate *cpmapp.TemplateHandler,
	importPublisher *rabbitmq.JobPublisherAdapter,
) *CostDataImportHandler {
	return &CostDataImportHandler{
		jobRepo:         jobRepo,
		storage:         storageSvc,
		cappExport:      cappExport,
		cappTemplate:    cappTemplate,
		cppExport:       cppExport,
		cppTemplate:     cppTemplate,
		cpmExport:       cpmExport,
		cpmTemplate:     cpmTemplate,
		importPublisher: importPublisher,
	}
}

// GetCostImportJob returns a single import job by ID.
func (h *CostDataImportHandler) GetCostImportJob(ctx context.Context, req *financev1.GetCostImportJobRequest) (*financev1.GetCostImportJobResponse, error) {
	job, err := h.jobRepo.GetByID(ctx, req.GetJobId())
	if err != nil {
		return &financev1.GetCostImportJobResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	proto, presignErr := h.importJobToProto(ctx, job)
	if presignErr != nil {
		return &financev1.GetCostImportJobResponse{Base: InternalErrorResponse(presignErr.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.GetCostImportJobResponse{
		Base: successResponse("OK"),
		Data: proto,
	}, nil
}

// ListCostImportJobs returns a paginated list of import jobs.
func (h *CostDataImportHandler) ListCostImportJobs(ctx context.Context, req *financev1.ListCostImportJobsRequest) (*financev1.ListCostImportJobsResponse, error) {
	page := int32(1)
	pageSize := int32(20)
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = req.Pagination.Page
		}
		if req.Pagination.PageSize > 0 {
			pageSize = req.Pagination.PageSize
		}
	}

	jobs, total, err := h.jobRepo.List(ctx, req.GetEntity(), req.GetStatus(), int(page), int(pageSize))
	if err != nil {
		return &financev1.ListCostImportJobsResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	items := make([]*financev1.CostImportJob, 0, len(jobs))
	for _, j := range jobs {
		p, presignErr := h.importJobToProto(ctx, j)
		if presignErr != nil {
			return &financev1.ListCostImportJobsResponse{Base: InternalErrorResponse(presignErr.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
		}
		items = append(items, p)
	}

	totalPages := int32(0)
	if pageSize > 0 {
		totalPages = safeIntToInt32(int((total + int64(pageSize) - 1) / int64(pageSize)))
	}

	return &financev1.ListCostImportJobsResponse{
		Base: successResponse("OK"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: page,
			PageSize:    pageSize,
			TotalItems:  total,
			TotalPages:  totalPages,
		},
	}, nil
}

// ImportCostApplicableParams uploads file to MinIO, creates a PENDING job, and publishes to RabbitMQ.
func (h *CostDataImportHandler) ImportCostApplicableParams(ctx context.Context, req *financev1.ImportCostApplicableParamsRequest) (*financev1.ImportCostApplicableParamsResponse, error) {
	jobID, err := h.enqueueImport(ctx, req.GetFileContent(), req.GetFileName(), costimportjob.EntityCAPP)
	if err != nil {
		return &financev1.ImportCostApplicableParamsResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.ImportCostApplicableParamsResponse{
		Base:  successResponse("CAPP import queued"),
		JobId: jobID,
	}, nil
}

// ImportCostProductParameters uploads file to MinIO, creates a PENDING job, and publishes to RabbitMQ.
func (h *CostDataImportHandler) ImportCostProductParameters(ctx context.Context, req *financev1.ImportCostProductParametersRequest) (*financev1.ImportCostProductParametersResponse, error) {
	jobID, err := h.enqueueImport(ctx, req.GetFileContent(), req.GetFileName(), costimportjob.EntityCPP)
	if err != nil {
		return &financev1.ImportCostProductParametersResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.ImportCostProductParametersResponse{
		Base:  successResponse("CPP import queued"),
		JobId: jobID,
	}, nil
}

// ExportCostApplicableParams exports CAPP data to Excel.
func (h *CostDataImportHandler) ExportCostApplicableParams(ctx context.Context, _ *financev1.ExportCostApplicableParamsRequest) (*financev1.ExportCostApplicableParamsResponse, error) {
	result, err := h.cappExport.Handle(ctx)
	if err != nil {
		return &financev1.ExportCostApplicableParamsResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.ExportCostApplicableParamsResponse{
		Base:        successResponse("CAPP exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// DownloadCostApplicableParamTemplate returns the CAPP import template.
func (h *CostDataImportHandler) DownloadCostApplicableParamTemplate(_ context.Context, _ *financev1.DownloadCostApplicableParamTemplateRequest) (*financev1.DownloadCostApplicableParamTemplateResponse, error) {
	result, err := h.cappTemplate.Handle()
	if err != nil {
		return &financev1.DownloadCostApplicableParamTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.DownloadCostApplicableParamTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ExportCostProductParameters exports CPP data to Excel.
func (h *CostDataImportHandler) ExportCostProductParameters(ctx context.Context, _ *financev1.ExportCostProductParametersRequest) (*financev1.ExportCostProductParametersResponse, error) {
	result, err := h.cppExport.Handle(ctx)
	if err != nil {
		return &financev1.ExportCostProductParametersResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.ExportCostProductParametersResponse{
		Base:        successResponse("CPP exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// DownloadCostProductParameterTemplate returns the CPP import template.
func (h *CostDataImportHandler) DownloadCostProductParameterTemplate(_ context.Context, _ *financev1.DownloadCostProductParameterTemplateRequest) (*financev1.DownloadCostProductParameterTemplateResponse, error) {
	result, err := h.cppTemplate.Handle()
	if err != nil {
		return &financev1.DownloadCostProductParameterTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.DownloadCostProductParameterTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// =============================================================================
// helpers
// =============================================================================

// enqueueImport uploads the file content to MinIO, creates a PENDING job,
// publishes it to RabbitMQ, and returns the job ID.
func (h *CostDataImportHandler) enqueueImport(ctx context.Context, fileContent []byte, _ /*fileName*/ string, entity string) (int64, error) {
	actor := getUserFromContext(ctx)
	requestingUserID, _ := GetUserIDFromCtx(ctx)
	fileKey := fmt.Sprintf("imports/%s/%s_%d.xlsx", entity, actor, time.Now().UnixNano())

	if h.storage != nil {
		if err := h.storage.PutObject(ctx, fileKey, bytes.NewReader(fileContent), int64(len(fileContent)),
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); err != nil {
			return 0, fmt.Errorf("upload import file: %w", err)
		}
	}

	job := costimportjob.NewJob(entity, fileKey, actor, requestingUserID)
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return 0, fmt.Errorf("create import job: %w", err)
	}

	if h.importPublisher != nil {
		if err := h.importPublisher.PublishImportJob(ctx, job.JobID(), entity, requestingUserID); err != nil {
			return 0, fmt.Errorf("publish import job: %w", err)
		}
	}

	return job.JobID(), nil
}

// importJobToProto maps a domain CostImportJob to the proto CostImportJob.
// If the job has an error file, it generates a presigned download URL.
func (h *CostDataImportHandler) importJobToProto(ctx context.Context, job *costimportjob.CostImportJob) (*financev1.CostImportJob, error) {
	errorFileURL := ""
	if job.ErrorFile() != "" && h.storage != nil {
		u, err := h.storage.PresignedGetURL(ctx, job.ErrorFile(), importPresignValidity, "import_errors.xlsx")
		if err != nil {
			return nil, fmt.Errorf("presign error file URL: %w", err)
		}
		errorFileURL = u
	}

	p := &financev1.CostImportJob{
		JobId:        job.JobID(),
		Entity:       job.Entity(),
		Status:       job.Status(),
		TotalRows:    safeIntToInt32(job.TotalRows()),
		Processed:    safeIntToInt32(job.Processed()),
		Success:      safeIntToInt32(job.Success()),
		Failed:       safeIntToInt32(job.Failed()),
		Skipped:      safeIntToInt32(job.Skipped()),
		ErrorFileUrl: errorFileURL,
		CreatedBy:    job.CreatedBy(),
		CreatedAt:    job.CreatedAt().Format(time.RFC3339),
	}

	if job.StartedAt() != nil {
		p.StartedAt = job.StartedAt().Format(time.RFC3339)
	}
	if job.CompletedAt() != nil {
		p.CompletedAt = job.CompletedAt().Format(time.RFC3339)
	}

	return p, nil
}
