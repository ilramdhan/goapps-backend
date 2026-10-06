package erpintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// TriggerSubtype enqueues a batch-less erp_integration job (attr_backfill,
// backtest). params is JSON-encoded as the job payload; one active
// erp_integration job per period is enforced like the batch steps.
func (h *StepTriggerHandler) TriggerSubtype(ctx context.Context, subtype, period string, params any, actor string) (*job.Execution, error) {
	if h.publisher == nil {
		return nil, ErrPublisherUnavailable
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: actor required", ErrInvalidJobParams)
	}
	active, err := h.jobs.HasActiveJob(ctx, job.TypeErpIntegration, period)
	if err != nil {
		return nil, fmt.Errorf("check active erp_integration job: %w", err)
	}
	if active {
		return nil, job.ErrDuplicateActiveJob
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode params: %w", err)
	}
	exec, err := job.NewExecution(job.TypeErpIntegration, subtype, period, actor, erpJobPriority, raw)
	if err != nil {
		return nil, fmt.Errorf("new erp_integration job: %w", err)
	}
	if err := h.jobs.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("persist erp_integration job: %w", err)
	}
	if err := h.publisher.PublishErpIntegration(ctx, exec.ID().String(), subtype, period, actor); err != nil {
		return nil, h.abort(ctx, exec, fmt.Errorf("publish erp_integration job: %w", err))
	}
	return exec, nil
}
