package erpintegration

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ErpMasterSyncPublisher publishes to finance.jobs.erp_master_sync
// (rabbitmq.JobPublisherAdapter).
type ErpMasterSyncPublisher interface {
	PublishErpMasterSync(ctx context.Context, jobID, subtype, createdBy string) error
}

// MasterSyncTriggerHandler enqueues a manual ERP master replica sync.
type MasterSyncTriggerHandler struct {
	jobs      job.Repository
	publisher ErpMasterSyncPublisher
}

// NewMasterSyncTriggerHandler constructs the handler; a nil publisher fails
// closed with ErrPublisherUnavailable.
func NewMasterSyncTriggerHandler(jobs job.Repository, publisher ErpMasterSyncPublisher) *MasterSyncTriggerHandler {
	return &MasterSyncTriggerHandler{jobs: jobs, publisher: publisher}
}

// Trigger creates the job row and publishes it. Only one active master sync
// job (period "") is allowed at a time.
func (h *MasterSyncTriggerHandler) Trigger(ctx context.Context, subtype, actor string) (*job.Execution, error) {
	if h.publisher == nil {
		return nil, ErrPublisherUnavailable
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: actor required", ErrInvalidJobParams)
	}
	active, err := h.jobs.HasActiveJob(ctx, job.TypeErpMasterSync, "")
	if err != nil {
		return nil, fmt.Errorf("check active erp_master_sync job: %w", err)
	}
	if active {
		return nil, job.ErrDuplicateActiveJob
	}
	exec, err := job.NewExecution(job.TypeErpMasterSync, subtype, "", actor, erpJobPriority, nil)
	if err != nil {
		return nil, fmt.Errorf("new erp_master_sync job: %w", err)
	}
	if err := h.jobs.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist erp_master_sync job: %w", err)
	}
	if err := h.publisher.PublishErpMasterSync(ctx, exec.ID().String(), subtype, actor); err != nil {
		if ferr := exec.Fail(err.Error()); ferr == nil {
			if uerr := h.jobs.UpdateStatus(ctx, exec); uerr != nil {
				log.Error().Err(uerr).Str("job_id", exec.ID().String()).Msg("mark aborted erp_master_sync job failed")
			}
		}
		return nil, fmt.Errorf("publish erp_master_sync job: %w", err)
	}
	return exec, nil
}
