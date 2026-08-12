package mbbatch

import (
	"context"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// triggeredByMBBatch labels cal_job rows created by this trigger, mirroring mbpush's
// "system:mb_batch" actor convention used in persistResult.
const triggeredByMBBatch = "system:mb_batch"

// TriggerResult is the outward-facing summary of one MB_BATCH trigger run, wrapping
// BatchResult with the cal_job tracking fields the gRPC handler needs to report.
type TriggerResult struct {
	JobID        int64
	Period       string
	SuccessCount int32
	FailedCount  int32
	RowCount     int32
	DurationMs   int64
	Errors       []BatchError
}

// TriggerHandler tracks an MB_BATCH run through the existing cal_job/JobRepository
// machinery (design doc §10 addendum), delegating the actual compute to Service.RunMBBatch.
// The job's cj_calculation_type is stored as ACTUAL — a nominal placeholder, since MB_BATCH
// computes all 3 calc types per MB and cal_job has no multi-type representation (design
// addendum leaves this an implementer choice; ACTUAL avoids a further migration).
type TriggerHandler struct {
	svc     *Service
	jobRepo costcalcdom.JobRepository
}

// NewTriggerHandler constructs a TriggerHandler.
func NewTriggerHandler(svc *Service, jobRepo costcalcdom.JobRepository) *TriggerHandler {
	return &TriggerHandler{svc: svc, jobRepo: jobRepo}
}

// Handle creates a cal_job row scoped MB_BATCH, drives it through
// PLANNING -> PROCESSING -> SUCCESS/PARTIAL_FAILED/FAILED, and runs the batch compute in between.
func (h *TriggerHandler) Handle(ctx context.Context, period, actor string) (*TriggerResult, error) {
	job, err := costcalcdom.NewJob(period, costcalcdom.CalcTypeActual, costcalcdom.ScopeMBBatch, nil, triggeredByMBBatch, actor)
	if err != nil {
		return nil, fmt.Errorf("new job: %w", err)
	}
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	if err := h.advance(ctx, job, job.MarkPlanning); err != nil {
		return nil, err
	}
	if err := h.advance(ctx, job, job.MarkProcessing); err != nil {
		return nil, err
	}

	batchResult, runErr := h.svc.RunMBBatch(ctx, period, job.ID())
	if runErr != nil {
		if compErr := h.failJob(ctx, job, runErr); compErr != nil {
			return nil, compErr
		}
		return nil, fmt.Errorf("run mb batch: %w", runErr)
	}

	succ := int(batchResult.MBCount)
	fail := len(batchResult.Errors)
	if err := job.MarkComplete(succ, fail, 0); err != nil {
		return nil, fmt.Errorf("mark complete: %w", err)
	}
	if err := h.jobRepo.UpdateCompletion(ctx, job.ID(), job.Status(), succ, fail, 0, job.DurationMs(), jsonOrNil(batchResult.Errors)); err != nil {
		return nil, fmt.Errorf("update job completion: %w", err)
	}

	return &TriggerResult{
		JobID:        job.ID(),
		Period:       batchResult.Period,
		SuccessCount: batchResult.MBCount,
		FailedCount:  int32(fail), //nolint:gosec // fail derives from len(), bounded by candidate count
		RowCount:     batchResult.RowCount,
		DurationMs:   job.DurationMs(),
		Errors:       batchResult.Errors,
	}, nil
}

// advance runs a Job lifecycle transition and persists the resulting status.
func (h *TriggerHandler) advance(ctx context.Context, job *costcalcdom.Job, transition func() error) error {
	if err := transition(); err != nil {
		return fmt.Errorf("job transition: %w", err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, job.ID(), job.Status()); err != nil {
		return fmt.Errorf("update job status %s: %w", job.Status(), err)
	}
	return nil
}

// failJob marks the job FAILED when RunMBBatch itself errored (e.g. advisory lock
// acquisition failure), rather than a per-MB compute failure already captured in BatchResult.
func (h *TriggerHandler) failJob(ctx context.Context, job *costcalcdom.Job, runErr error) error {
	if err := job.MarkComplete(0, 0, 1); err != nil {
		return fmt.Errorf("mark complete after run error %w: %w", runErr, err)
	}
	errSummary := jsonOrNil(BatchError{MBHID: "", Error: runErr.Error()})
	if err := h.jobRepo.UpdateCompletion(ctx, job.ID(), job.Status(), 0, 0, 1, job.DurationMs(), errSummary); err != nil {
		return fmt.Errorf("update job completion after run error %w: %w", runErr, err)
	}
	return nil
}
