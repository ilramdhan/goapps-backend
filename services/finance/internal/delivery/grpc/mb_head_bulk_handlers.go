// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"encoding/json"
	"errors"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbheadbulk"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"

	"github.com/google/uuid"
)

// bulkTransitionQueueUnavailable is returned by all three Bulk MB Head
// Regenerate mutation RPCs when WithBulkTransition was never called (bulk
// transition dependencies never wired) — goconst: appears 3+ times below.
const bulkTransitionQueueUnavailable = "bulk transition queue unavailable"

// mbBulkErrToBase maps Bulk MB Head Regenerate domain/application errors onto
// client-meaningful BaseResponse status codes, following the
// mbHeadLockErrToBase / requestErrToBase pattern already used across this
// package.
//
// mbheadbulk.ErrPublisherUnavailable gets its own 503: it is not a bad request
// or a data conflict, it means the finance service itself cannot reach
// RabbitMQ — the caller should retry later, not change the request.
// job.ErrNotFound is mapped explicitly (rather than falling through to
// domainErrorToBaseResponse's string-matching) so a bad job_id reliably reads
// as 404 regardless of the exact wording of job.ErrNotFound's message.
func mbBulkErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbheadbulk.ErrPublisherUnavailable):
		return ErrorResponse("503", err.Error())
	case errors.Is(err, job.ErrNotFound):
		return NotFoundResponse(err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// bulkMBHeadJobInfoFromExecution maps a job.Execution (parent batch job) onto the
// proto's BulkMBHeadJobInfo/GetBulkMBHeadJobStatusResponse shape. Unlike
// productCostSheetExportJobInfoFromExecution's template (cost_calc_handler.go),
// there is no IsBatch field on this proto message — every bulk MB head job IS a
// batch parent, so the field would be redundant.
func bulkMBHeadJobInfoFromExecution(exec *job.Execution) *financev1.BulkMBHeadJobInfo {
	return &financev1.BulkMBHeadJobInfo{
		JobId:             exec.ID().String(),
		JobCode:           exec.Code().String(),
		Status:            string(exec.Status()),
		TotalChildren:     safeIntToInt32(exec.TotalChildren()),
		CompletedChildren: safeIntToInt32(exec.CompletedChildren()),
		FailedChildren:    safeIntToInt32(exec.FailedChildren()),
	}
}

// bulkTransitionChildParams mirrors mbheadbulk.buildParams' JSON shape, just
// enough to read mbh_id back off a failed child job row.
type bulkTransitionChildParams struct {
	MbhID string `json:"mbh_id"`
}

// BulkForceUnvalidateMBHead queues an async job that force-unvalidates (VALIDATED
// → DRAFT, bypassing the ordinary Submit/Approve gate) every MB Head in
// req.MbhIds, one child job per head. Super Admin only — enforced by
// auth_interceptor.go's permission map, not here.
func (h *MBHeadHandler) BulkForceUnvalidateMBHead(
	ctx context.Context, req *financev1.BulkForceUnvalidateMBHeadRequest,
) (*financev1.BulkForceUnvalidateMBHeadResponse, error) {
	if h.bulkTransitionHandler == nil {
		return &financev1.BulkForceUnvalidateMBHeadResponse{Base: ErrorResponse("503", bulkTransitionQueueUnavailable)}, nil
	}

	result, err := h.bulkTransitionHandler.Handle(ctx, mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs:    req.GetMbhIds(),
		Action:    mbheadbulk.ActionForceUnvalidate,
		Reason:    req.GetReason(),
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.BulkForceUnvalidateMBHeadResponse{Base: mbBulkErrToBase(err)}, nil
	}

	return &financev1.BulkForceUnvalidateMBHeadResponse{
		Base: successResponse("Bulk MB head force-unvalidate job queued successfully"),
		Data: bulkMBHeadJobInfoFromExecution(result.Execution),
	}, nil
}

// BulkSubmitMBHead queues an async job that submits (DRAFT → SUBMITTED) every MB
// Head in req.MbhIds, one child job per head.
func (h *MBHeadHandler) BulkSubmitMBHead(
	ctx context.Context, req *financev1.BulkSubmitMBHeadRequest,
) (*financev1.BulkSubmitMBHeadResponse, error) {
	if h.bulkTransitionHandler == nil {
		return &financev1.BulkSubmitMBHeadResponse{Base: ErrorResponse("503", bulkTransitionQueueUnavailable)}, nil
	}

	result, err := h.bulkTransitionHandler.Handle(ctx, mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs:    req.GetMbhIds(),
		Action:    mbheadbulk.ActionSubmit,
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.BulkSubmitMBHeadResponse{Base: mbBulkErrToBase(err)}, nil
	}

	return &financev1.BulkSubmitMBHeadResponse{
		Base: successResponse("Bulk MB head submit job queued successfully"),
		Data: bulkMBHeadJobInfoFromExecution(result.Execution),
	}, nil
}

// BulkValidateMBHead queues an async job that validates (SUBMITTED/APPROVED →
// VALIDATED) every MB Head in req.MbhIds, one child job per head. This is the
// step that regenerates cost_product_master/cost_route_*/CAPP/CPP/MB Spin data —
// the entire point of the Bulk MB Head Regenerate feature.
func (h *MBHeadHandler) BulkValidateMBHead(
	ctx context.Context, req *financev1.BulkValidateMBHeadRequest,
) (*financev1.BulkValidateMBHeadResponse, error) {
	if h.bulkTransitionHandler == nil {
		return &financev1.BulkValidateMBHeadResponse{Base: ErrorResponse("503", bulkTransitionQueueUnavailable)}, nil
	}

	result, err := h.bulkTransitionHandler.Handle(ctx, mbheadbulk.RequestBulkTransitionCommand{
		MBHIDs:    req.GetMbhIds(),
		Action:    mbheadbulk.ActionValidate,
		CreatedBy: getUserFromContext(ctx),
	})
	if err != nil {
		return &financev1.BulkValidateMBHeadResponse{Base: mbBulkErrToBase(err)}, nil
	}

	return &financev1.BulkValidateMBHeadResponse{
		Base: successResponse("Bulk MB head validate job queued successfully"),
		Data: bulkMBHeadJobInfoFromExecution(result.Execution),
	}, nil
}

// GetBulkMBHeadJobStatus reports the current progress of a Bulk MB Head Regenerate
// batch job by ID — the parent job's own status plus its child counters, exactly
// like CostCalcHandler.GetProductCostSheetExportJobStatus.
func (h *MBHeadHandler) GetBulkMBHeadJobStatus(
	ctx context.Context, req *financev1.GetBulkMBHeadJobStatusRequest,
) (*financev1.GetBulkMBHeadJobStatusResponse, error) {
	if h.bulkJobRepo == nil {
		return &financev1.GetBulkMBHeadJobStatusResponse{Base: ErrorResponse("503", "job tracking unavailable")}, nil
	}

	jobID, err := uuid.Parse(req.GetJobId())
	if err != nil {
		return &financev1.GetBulkMBHeadJobStatusResponse{Base: invalidIDResponse("job_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	exec, err := h.bulkJobRepo.GetByID(ctx, jobID)
	if err != nil {
		return &financev1.GetBulkMBHeadJobStatusResponse{Base: mbBulkErrToBase(err)}, nil
	}

	info := bulkMBHeadJobInfoFromExecution(exec)
	return &financev1.GetBulkMBHeadJobStatusResponse{
		Base:              successResponse("OK"),
		JobId:             info.GetJobId(),
		JobCode:           info.GetJobCode(),
		Status:            info.GetStatus(),
		TotalChildren:     info.GetTotalChildren(),
		CompletedChildren: info.GetCompletedChildren(),
		FailedChildren:    info.GetFailedChildren(),
	}, nil
}

// ListBulkMBHeadJobFailures lists every MB Head that failed within a Bulk MB Head
// Regenerate batch job, so the Super Admin can see exactly which heads need
// attention instead of only a failed-count.
//
// There is no dedicated failure-detail table: each failed child job.Execution row
// already carries everything needed — mbh_id from Params() (the same JSON
// mbheadbulk.buildParams wrote when the batch was queued) and the failure text
// from ErrorMessage() (written by the worker's markFailed, see
// internal/worker/mb_bulk_transition_handler.go). mb_costing is resolved
// best-effort via mbhead.Repository at read time since the child job itself never
// stores it — a lookup failure leaves the field empty rather than failing the
// whole list.
func (h *MBHeadHandler) ListBulkMBHeadJobFailures(
	ctx context.Context, req *financev1.ListBulkMBHeadJobFailuresRequest,
) (*financev1.ListBulkMBHeadJobFailuresResponse, error) {
	if h.bulkJobRepo == nil {
		return &financev1.ListBulkMBHeadJobFailuresResponse{Base: ErrorResponse("503", "job tracking unavailable")}, nil
	}

	jobID, err := uuid.Parse(req.GetJobId())
	if err != nil {
		return &financev1.ListBulkMBHeadJobFailuresResponse{Base: invalidIDResponse("job_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	children, err := h.bulkJobRepo.ListChildren(ctx, jobID)
	if err != nil {
		return &financev1.ListBulkMBHeadJobFailuresResponse{Base: mbBulkErrToBase(err)}, nil
	}

	failures := make([]*financev1.BulkMBHeadJobFailure, 0, len(children))
	for _, child := range children {
		if child.Status() != job.StatusFailed {
			continue
		}
		failures = append(failures, h.bulkChildToFailure(ctx, child))
	}

	return &financev1.ListBulkMBHeadJobFailuresResponse{
		Base:     successResponse("OK"),
		Failures: failures,
	}, nil
}

// bulkChildToFailure turns one failed child job.Execution into a
// BulkMBHeadJobFailure, resolving mb_costing best-effort from the MB Head repo.
func (h *MBHeadHandler) bulkChildToFailure(ctx context.Context, child *job.Execution) *financev1.BulkMBHeadJobFailure {
	failure := &financev1.BulkMBHeadJobFailure{
		ErrorMessage: child.ErrorMessage(),
	}

	var params bulkTransitionChildParams
	if err := json.Unmarshal(child.Params(), &params); err != nil {
		return failure
	}
	failure.MbhId = params.MbhID

	if h.bulkMBHeadRepo == nil || params.MbhID == "" {
		return failure
	}
	mbhID, err := uuid.Parse(params.MbhID)
	if err != nil {
		return failure
	}
	entity, err := h.bulkMBHeadRepo.GetByID(ctx, mbhID)
	if err != nil {
		return failure
	}
	failure.MbCosting = entity.MBCosting()
	return failure
}
