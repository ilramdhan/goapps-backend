// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costbulkimport"
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

// GetImportUploadURL issues a presigned PUT URL so the browser can upload a large
// import file (xlsx or zipped CSV) straight to object storage, bypassing the BFF
// and the gRPC message path. The returned object_key is handed back to
// StartCostingImport once the upload completes.
func (h *CostDataImportHandler) GetImportUploadURL(ctx context.Context, req *financev1.GetImportUploadURLRequest) (*financev1.GetImportUploadURLResponse, error) {
	if h.storage == nil {
		return &financev1.GetImportUploadURLResponse{Base: InternalErrorResponse("storage unavailable")}, nil
	}

	objectKey := buildImportObjectKey(req.GetKind(), req.GetFileName())
	uploadURL, err := h.storage.PresignPutURL(ctx, objectKey, importPresignValidity)
	if err != nil {
		return &financev1.GetImportUploadURLResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.GetImportUploadURLResponse{
		Base:             successResponse("OK"),
		UploadUrl:        uploadURL,
		ObjectKey:        objectKey,
		ExpiresInSeconds: int64(importPresignValidity.Seconds()),
	}, nil
}

// StartCostingImport enqueues an async ETL import job for an already-uploaded
// object (identified by object_key from GetImportUploadURL). It creates a PENDING
// job whose file_key is the object key and publishes one RabbitMQ message; the
// worker streams the object from storage into staging and resolves it set-based.
func (h *CostDataImportHandler) StartCostingImport(ctx context.Context, req *financev1.StartCostingImportRequest) (*financev1.StartCostingImportResponse, error) {
	entity, err := importKindToEntity(req.GetKind())
	if err != nil {
		return &financev1.StartCostingImportResponse{Base: BadRequestResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	actor := getUserFromContext(ctx)
	requestingUserID, _ := GetUserIDFromCtx(ctx)

	job := costimportjob.NewJob(entity, req.GetObjectKey(), actor, requestingUserID)
	if createErr := h.jobRepo.Create(ctx, job); createErr != nil {
		return &financev1.StartCostingImportResponse{Base: InternalErrorResponse(createErr.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	if h.importPublisher != nil {
		if pubErr := h.importPublisher.PublishImportJob(ctx, job.JobID(), entity, requestingUserID); pubErr != nil {
			return &financev1.StartCostingImportResponse{Base: InternalErrorResponse(pubErr.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
		}
	}

	return &financev1.StartCostingImportResponse{
		Base:   successResponse("Import queued"),
		JobId:  job.JobID(),
		Status: job.Status(),
	}, nil
}

// ExportBulkProductRouting creates an async export job for bulk product routing data
// and publishes it to RabbitMQ for the worker to process.
func (h *CostDataImportHandler) ExportBulkProductRouting(ctx context.Context, req *financev1.ExportBulkProductRoutingRequest) (*financev1.ExportBulkProductRoutingResponse, error) {
	actor := getUserFromContext(ctx)
	requestingUserID, _ := GetUserIDFromCtx(ctx)

	exportReq := costbulkimport.ExportRequest{
		ProductTypeCodes: req.GetProductTypeCodes(),
		IncludeRouting:   req.GetIncludeRouting(),
		ActiveOnly:       req.GetActiveOnly(),
		Actor:            actor,
		ProductSysIDs:    req.GetProductSysIds(),
	}

	fileKey, err := marshalExportRequestKey(exportReq)
	if err != nil {
		return &financev1.ExportBulkProductRoutingResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	job := costimportjob.NewJob(costimportjob.EntityBulkProductRoutingExport, fileKey, actor, requestingUserID)
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return &financev1.ExportBulkProductRoutingResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	if err := h.importPublisher.PublishImportJob(ctx, job.JobID(), costimportjob.EntityBulkProductRoutingExport, requestingUserID); err != nil {
		return &financev1.ExportBulkProductRoutingResponse{Base: InternalErrorResponse(err.Error())}, nil //nolint:nilerr // intentional BaseResponse pattern
	}

	return &financev1.ExportBulkProductRoutingResponse{
		Base:  successResponse("Bulk product routing export queued"),
		JobId: job.JobID(),
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
// If the job has a result/error file, it generates a presigned download URL.
func (h *CostDataImportHandler) importJobToProto(ctx context.Context, job *costimportjob.CostImportJob) (*financev1.CostImportJob, error) {
	errorFileURL := ""
	if job.ErrorFile() != "" && h.storage != nil {
		filename := downloadFilename(job.Entity())
		u, err := h.storage.PresignedGetURL(ctx, job.ErrorFile(), importPresignValidity, filename)
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

// marshalExportRequestKey encodes an ExportRequest as a JSON string for storage
// in the job's file_key field. The worker decodes it to reconstruct the request.
func marshalExportRequestKey(req costbulkimport.ExportRequest) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal export request: %w", err)
	}
	return string(b), nil
}

// downloadFilename returns the suggested download filename for a job's result/error file.
// Export jobs produce an Excel workbook; import jobs produce an error report.
func downloadFilename(entity string) string {
	if entity == costimportjob.EntityBulkProductRoutingExport {
		return "bulk_product_routing_export.xlsx"
	}
	return "import_errors.xlsx"
}

// importKindToEntity maps the proto ImportKind to the internal cost-import-job
// entity key the worker dispatches on.
func importKindToEntity(kind financev1.ImportKind) (string, error) {
	switch kind {
	case financev1.ImportKind_IMPORT_KIND_PRODUCT_ROUTING:
		return costimportjob.EntityBulkProductRouting, nil
	case financev1.ImportKind_IMPORT_KIND_PARAMS_ONLY:
		return costimportjob.EntityBulkParamsOnly, nil
	case financev1.ImportKind_IMPORT_KIND_UNSPECIFIED:
		return "", fmt.Errorf("import kind must be specified")
	default:
		return "", fmt.Errorf("unsupported import kind: %s", kind)
	}
}

// buildImportObjectKey returns the storage object key for a presigned import
// upload. The uploaded file's extension is preserved (defaulting to .zip) so the
// ETL worker can detect the container format (.xlsx / .zip / .csv) from the key.
func buildImportObjectKey(kind financev1.ImportKind, fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext == "" {
		ext = ".zip"
	}
	entity, err := importKindToEntity(kind)
	if err != nil {
		entity = "unknown"
	}
	return fmt.Sprintf("imports/%s/%s%s", entity, uuid.NewString(), ext)
}
