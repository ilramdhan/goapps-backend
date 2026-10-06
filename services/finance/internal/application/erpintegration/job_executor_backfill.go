package erpintegration

// job_executor_backfill.go routes the erp_integration attr_backfill job
// (P3-T6, design Part 2 §10 subtype attr_backfill) to the backfill handler.
// Unlike batch steps it carries no batch_id: params are {dry_run?, period?}
// with dry_run defaulting to true (BackfillModeFromDryRun).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// AttrBackfillJobParams is the attr_backfill job params payload.
type AttrBackfillJobParams struct {
	DryRun *bool  `json:"dry_run,omitempty"`
	Period string `json:"period,omitempty"`
}

// ParseAttrBackfillJobParams decodes the params; empty params mean a dry run
// over every period.
func ParseAttrBackfillJobParams(raw json.RawMessage) (AttrBackfillJobParams, error) {
	var p AttrBackfillJobParams
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return p, nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%w: %w", ErrInvalidJobParams, err)
	}
	p.Period = strings.TrimSpace(p.Period)
	return p, nil
}

// WithAttrBackfill attaches the attribute backfill handler.
func (e *JobExecutor) WithAttrBackfill(h *BackfillAttributesHandler) *JobExecutor {
	e.backfill = h
	return e
}

// runAttrBackfill runs the attr_backfill job. The params period wins over the
// job period.
func (e *JobExecutor) runAttrBackfill(ctx context.Context, exec *job.Execution, progress ProgressFunc) error {
	p, err := ParseAttrBackfillJobParams(exec.Params())
	if err != nil {
		return e.failJob(ctx, exec, 0, err)
	}
	if e.backfill == nil {
		return e.failJob(ctx, exec, 0, domain.ErrAttrBackfillNotConfigured)
	}
	period := p.Period
	if period == "" {
		period = strings.TrimSpace(exec.Period())
	}
	progress.report(ctx, progressRead)
	rep, err := e.backfill.Handle(ctx, BackfillAttributesCommand{
		Period: period, Mode: BackfillModeFromDryRun(p.DryRun), Actor: exec.CreatedBy(),
	})
	if err != nil {
		return e.failJob(ctx, exec, 0, err)
	}
	progress.report(ctx, progressPersist)
	raw, err := json.Marshal(rep)
	if err != nil {
		return e.failJob(ctx, exec, 0, fmt.Errorf("encode result: %w", err))
	}
	if err := exec.Complete(raw); err != nil {
		return fmt.Errorf("complete erp_integration job %s: %w", exec.ID(), err)
	}
	if err := e.jobs.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("mark erp_integration job %s success: %w", exec.ID(), err)
	}
	return nil
}
