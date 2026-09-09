// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/productparambulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// productParamBulkQueueUnavailable is returned by BulkEditProductParams when
// WithSubmitHandler was never called (bulk edit dependencies never wired) —
// goconst: mirrors bulkTransitionQueueUnavailable's role in mb_head_bulk_handlers.go.
const productParamBulkQueueUnavailable = "bulk product param edit queue unavailable"

// jobTrackingUnavailable is returned by GetBulkProductParamJobStatus and
// ListBulkProductParamJobFailures when WithSubmitHandler was never called.
const jobTrackingUnavailable = "job tracking unavailable"

// CostProductParamBulkHandler implements financev1.CostProductParamBulkServiceServer
// — the Bulk Edit Product Params (F4) submit/status/failures RPCs. This is a
// SEPARATE proto service from CostProductParameterService (confirmed via the
// generated grpc.pb.go — each requires its own
// mustEmbedUnimplementedXxxServiceServer()), so it gets its own dedicated
// handler struct rather than extending CostProductParameterHandler, mirroring
// the fact that its auth_interceptor.go permission mappings are also wired
// independently.
type CostProductParamBulkHandler struct {
	financev1.UnimplementedCostProductParamBulkServiceServer
	submitHandler *productparambulk.RequestBulkEditHandler
	jobRepo       job.Repository
	// productRepo is used ONLY for the best-effort product_code lookup in
	// ListBulkProductParamJobFailures — it is the same repo instance already
	// wired to every other handler, not a second connection. A nil productRepo
	// leaves BulkProductParamJobFailure.ProductCode empty rather than failing
	// the whole list.
	productRepo costproductmaster.Repository
}

// NewCostProductParamBulkHandler constructs the handler. All fields are nil
// until WithSubmitHandler is called — every RPC degrades to a clean 503
// rather than a nil dereference until then.
func NewCostProductParamBulkHandler() *CostProductParamBulkHandler {
	return &CostProductParamBulkHandler{}
}

// WithSubmitHandler attaches the Bulk Edit Product Params dependencies and
// returns the handler for chaining. jobRepo and productRepo are the SAME
// instances already wired to the rest of the server — nothing new is
// connected here.
func (h *CostProductParamBulkHandler) WithSubmitHandler(
	submitHandler *productparambulk.RequestBulkEditHandler, jobRepo job.Repository, productRepo costproductmaster.Repository,
) *CostProductParamBulkHandler {
	h.submitHandler = submitHandler
	h.jobRepo = jobRepo
	h.productRepo = productRepo
	return h
}

// productParamBulkErrToBase maps Bulk Edit Product Params domain/application
// errors onto client-meaningful BaseResponse status codes, mirroring
// mbBulkErrToBase's pattern.
func productParamBulkErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, productparambulk.ErrPublisherUnavailable):
		return ErrorResponse("503", err.Error())
	case errors.Is(err, job.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, productparambulk.ErrProductNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, productparambulk.ErrNoProducts),
		errors.Is(err, productparambulk.ErrTooManyProducts),
		errors.Is(err, productparambulk.ErrNoOperations),
		errors.Is(err, productparambulk.ErrInvalidOperation):
		return ErrorResponse("400", err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// bulkParamJobInfoFromExecution maps a job.Execution (parent batch job) onto
// the proto's BulkParamJobInfo shape, mirroring bulkMBHeadJobInfoFromExecution.
func bulkParamJobInfoFromExecution(exec *job.Execution) *financev1.BulkParamJobInfo {
	return &financev1.BulkParamJobInfo{
		JobId:             exec.ID().String(),
		JobCode:           exec.Code().String(),
		Status:            bulkParamJobStatusString(exec),
		TotalChildren:     safeIntToInt32(exec.TotalChildren()),
		CompletedChildren: safeIntToInt32(exec.CompletedChildren()),
		FailedChildren:    safeIntToInt32(exec.FailedChildren()),
	}
}

// bulkParamJobStatusString translates the parent job's internal job.Status
// onto the QUEUED/PROCESSING/DONE/FAILED/PARTIAL vocabulary, mirroring
// bulkMBHeadJobStatusString's rationale exactly.
func bulkParamJobStatusString(exec *job.Execution) string {
	switch exec.Status() {
	case job.StatusSuccess:
		if exec.FailedChildren() > 0 {
			return "PARTIAL"
		}
		return "DONE"
	case job.StatusFailed:
		return "FAILED"
	default:
		return string(exec.Status()) // QUEUED / PROCESSING
	}
}

// bulkParamChildParams mirrors productparambulk's childParams JSON shape
// (that type is unexported there), just enough to read product_sys_id back
// off a failed child job row.
type bulkParamChildParams struct {
	ProductSysID int64 `json:"product_sys_id"`
}

// BulkEditProductParams queues an async job that applies req.Operations to
// every product in req.ProductSysIds, one child job per product.
func (h *CostProductParamBulkHandler) BulkEditProductParams(
	ctx context.Context, req *financev1.BulkEditProductParamsRequest,
) (*financev1.BulkEditProductParamsResponse, error) {
	if h.submitHandler == nil {
		return &financev1.BulkEditProductParamsResponse{Base: ErrorResponse("503", productParamBulkQueueUnavailable)}, nil
	}

	operations, err := operationsFromProto(req.GetOperations())
	if err != nil {
		return &financev1.BulkEditProductParamsResponse{Base: ErrorResponse("400", err.Error())}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	result, err := h.submitHandler.Handle(ctx, productparambulk.RequestBulkEditCommand{
		ProductSysIDs:         req.GetProductSysIds(),
		Operations:            operations,
		SkipMissingApplicable: req.GetSkipMissingApplicable(),
		CreatedBy:             getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.BulkEditProductParamsResponse{Base: productParamBulkErrToBase(err)}, nil
	}

	return &financev1.BulkEditProductParamsResponse{
		Base: successResponse("Bulk edit product params job queued successfully"),
		Data: bulkParamJobInfoFromExecution(result.Execution),
	}, nil
}

// operationsFromProto converts the proto's oneof-based BulkParamOperation
// list into productparambulk.OperationDTO. ValueNumeric/ValueText are
// treated as "unset" when empty — mirroring UpsertParamValueOp's own
// convention where exactly one of value_numeric/value_text/has_value_flag is
// meaningful — and ValueFlag is set from GetHasValueFlag()/GetValueFlag()
// rather than from a zero-value bool.
func operationsFromProto(ops []*financev1.BulkParamOperation) ([]productparambulk.OperationDTO, error) {
	dtos := make([]productparambulk.OperationDTO, 0, len(ops))
	for i, op := range ops {
		dto, err := operationFromProto(op)
		if err != nil {
			return nil, fmt.Errorf("operations[%d]: %w", i, err)
		}
		dtos = append(dtos, dto)
	}
	return dtos, nil
}

// operationFromProto converts one BulkParamOperation oneof into an OperationDTO.
func operationFromProto(op *financev1.BulkParamOperation) (productparambulk.OperationDTO, error) {
	switch v := op.GetOp().(type) {
	case *financev1.BulkParamOperation_AddApplicable:
		add := v.AddApplicable
		displayOrder := add.GetDisplayOrder()
		return productparambulk.OperationDTO{
			Kind:         productparambulk.OpAddApplicable,
			ParamID:      add.GetParamId(),
			IsRequired:   add.GetIsRequired(),
			DisplayOrder: &displayOrder,
		}, nil
	case *financev1.BulkParamOperation_RemoveApplicable:
		return productparambulk.OperationDTO{
			Kind:    productparambulk.OpRemoveApplicable,
			ParamID: v.RemoveApplicable.GetParamId(),
		}, nil
	case *financev1.BulkParamOperation_UpsertValue:
		return upsertValueOperationFromProto(v.UpsertValue), nil
	default:
		return productparambulk.OperationDTO{}, fmt.Errorf("%w: operation must set exactly one of add_applicable/remove_applicable/upsert_value", productparambulk.ErrInvalidOperation)
	}
}

// upsertValueOperationFromProto converts one UpsertParamValueOp into an
// OperationDTO, resolving exactly one of value_numeric/value_text/value_flag.
func upsertValueOperationFromProto(op *financev1.UpsertParamValueOp) productparambulk.OperationDTO {
	dto := productparambulk.OperationDTO{
		Kind:    productparambulk.OpUpsertValue,
		ParamID: op.GetParamId(),
	}
	switch {
	case op.GetHasValueFlag():
		flag := op.GetValueFlag()
		dto.ValueFlag = &flag
	case op.GetValueNumeric() != "":
		numeric := op.GetValueNumeric()
		dto.ValueNumeric = &numeric
	case op.GetValueText() != "":
		text := op.GetValueText()
		dto.ValueText = &text
	}
	return dto
}

// GetBulkProductParamJobStatus reports the current progress of a Bulk Edit
// Product Params batch job by ID.
func (h *CostProductParamBulkHandler) GetBulkProductParamJobStatus(
	ctx context.Context, req *financev1.GetBulkProductParamJobStatusRequest,
) (*financev1.GetBulkProductParamJobStatusResponse, error) {
	if h.jobRepo == nil {
		return &financev1.GetBulkProductParamJobStatusResponse{Base: ErrorResponse("503", jobTrackingUnavailable)}, nil
	}

	jobID, err := uuid.Parse(req.GetJobId())
	if err != nil {
		return &financev1.GetBulkProductParamJobStatusResponse{Base: invalidIDResponse("job_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return &financev1.GetBulkProductParamJobStatusResponse{Base: productParamBulkErrToBase(err)}, nil
	}

	info := bulkParamJobInfoFromExecution(exec)
	return &financev1.GetBulkProductParamJobStatusResponse{
		Base:              successResponse("OK"),
		JobId:             info.GetJobId(),
		JobCode:           info.GetJobCode(),
		Status:            info.GetStatus(),
		TotalChildren:     info.GetTotalChildren(),
		CompletedChildren: info.GetCompletedChildren(),
		FailedChildren:    info.GetFailedChildren(),
	}, nil
}

// ListBulkProductParamJobFailures lists every product that hard-failed within
// a Bulk Edit Product Params batch job. A skipped operation
// (skip_missing_applicable=true) is NOT a failure — see
// worker.ProductParamBulkHandler.markCompleted's doc comment — so only
// children with job.StatusFailed appear here, exactly like
// ListBulkMBHeadJobFailures.
func (h *CostProductParamBulkHandler) ListBulkProductParamJobFailures(
	ctx context.Context, req *financev1.ListBulkProductParamJobFailuresRequest,
) (*financev1.ListBulkProductParamJobFailuresResponse, error) {
	if h.jobRepo == nil {
		return &financev1.ListBulkProductParamJobFailuresResponse{Base: ErrorResponse("503", jobTrackingUnavailable)}, nil
	}

	jobID, err := uuid.Parse(req.GetJobId())
	if err != nil {
		return &financev1.ListBulkProductParamJobFailuresResponse{Base: invalidIDResponse("job_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	children, err := h.jobRepo.ListChildren(ctx, jobID)
	if err != nil {
		return &financev1.ListBulkProductParamJobFailuresResponse{Base: productParamBulkErrToBase(err)}, nil
	}

	failures := make([]*financev1.BulkProductParamJobFailure, 0, len(children))
	for _, child := range children {
		if child.Status() != job.StatusFailed {
			continue
		}
		failures = append(failures, h.bulkChildToFailure(ctx, child))
	}

	return &financev1.ListBulkProductParamJobFailuresResponse{
		Base:     successResponse("OK"),
		Failures: failures,
	}, nil
}

// bulkChildToFailure turns one failed child job.Execution into a
// BulkProductParamJobFailure, resolving product_code best-effort from
// costproductmaster.Repository.
func (h *CostProductParamBulkHandler) bulkChildToFailure(ctx context.Context, child *job.Execution) *financev1.BulkProductParamJobFailure {
	failure := &financev1.BulkProductParamJobFailure{
		ErrorMessage: child.ErrorMessage(),
	}

	var params bulkParamChildParams
	if err := json.Unmarshal(child.Params(), &params); err != nil {
		return failure
	}
	failure.ProductSysId = params.ProductSysID

	if h.productRepo == nil || params.ProductSysID == 0 {
		return failure
	}
	product, err := h.productRepo.GetBySysID(ctx, params.ProductSysID)
	if err != nil {
		return failure
	}
	failure.ProductCode = product.ProductCode()
	return failure
}
