// Package rmcost — V2 worker driver. Iterates groups and runs the V2 engine
// per group, then writes a job-execution status as the V1 path does.
package rmcost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ExecuteHandlerV2 is the V2 worker entrypoint.
type ExecuteHandlerV2 struct {
	jobRepo    job.Repository
	groupRepo  rmgroup.Repository
	calculator *CalculateHandlerV2
	logger     zerolog.Logger
}

// NewExecuteHandlerV2 builds the V2 worker handler.
func NewExecuteHandlerV2(
	jobRepo job.Repository,
	groupRepo rmgroup.Repository,
	calculator *CalculateHandlerV2,
	logger zerolog.Logger,
) *ExecuteHandlerV2 {
	return &ExecuteHandlerV2{jobRepo: jobRepo, groupRepo: groupRepo, calculator: calculator, logger: logger}
}

// Execute drives the job lifecycle around the V2 calc handler.
func (h *ExecuteHandlerV2) Execute(ctx context.Context, cmd ExecuteCommand) error {
	exec, err := h.jobRepo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", cmd.JobID, err)
	}
	if exec.Status().IsTerminal() {
		h.logger.Info().Str("job_id", cmd.JobID.String()).Str("status", exec.Status().String()).
			Msg("Skipping rm cost v2 job — already terminal")
		return nil
	}
	if err := exec.Start(); err != nil {
		return fmt.Errorf("start job %s: %w", cmd.JobID, err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	h.logger.Info().Str("job_id", cmd.JobID.String()).Str("period", cmd.Period).Msg("Starting RM cost V2 job")

	processed, skipped, calcErr := h.runForTargets(ctx, cmd)
	if calcErr != nil {
		return h.failJob(ctx, exec, calcErr)
	}
	return h.completeJob(ctx, exec, cmd.Period, processed, skipped)
}

func (h *ExecuteHandlerV2) runForTargets(ctx context.Context, cmd ExecuteCommand) (int, int, error) {
	heads, err := h.resolveTargets(ctx, cmd.GroupHeadID)
	if err != nil {
		return 0, 0, err
	}
	processed, skipped := 0, 0
	for _, head := range heads {
		if !head.IsActive() || head.IsDeleted() {
			skipped++
			continue
		}
		_, err := h.calculator.HandleOneGroup(ctx, head.ID(), cmd.Period, cmd.CalculatedBy)
		if err != nil {
			return processed, skipped, fmt.Errorf("calc head %s: %w", head.Code(), err)
		}
		processed++
	}
	return processed, skipped, nil
}

// resolveTargets mirrors V1 logic — single head or paginated list of active heads.
func (h *ExecuteHandlerV2) resolveTargets(ctx context.Context, headID *uuid.UUID) ([]*rmgroup.Head, error) {
	if headID != nil {
		head, err := h.groupRepo.GetHeadByID(ctx, *headID)
		if err != nil {
			return nil, fmt.Errorf("load head %s: %w", *headID, err)
		}
		return []*rmgroup.Head{head}, nil
	}
	active := true
	page := 1
	const pageSize = 100
	var all []*rmgroup.Head
	for {
		filter := rmgroup.ListFilter{IsActive: &active, Page: page, PageSize: pageSize, SortBy: "code", SortOrder: "asc"}
		filter.Validate()
		heads, total, err := h.groupRepo.ListHeads(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("list heads page %d: %w", page, err)
		}
		all = append(all, heads...)
		if int64(len(all)) >= total || len(heads) == 0 {
			break
		}
		page++
	}
	return all, nil
}

func (h *ExecuteHandlerV2) completeJob(ctx context.Context, exec *job.Execution, period string, processed, skipped int) error {
	summary, err := json.Marshal(map[string]any{
		"period":    period,
		"processed": processed,
		"skipped":   skipped,
		"engine":    "v2",
	})
	if err != nil {
		return fmt.Errorf("marshal rm cost summary: %w", err)
	}
	if err := exec.Complete(summary); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to success: %w", err)
	}
	h.logger.Info().Str("job_id", exec.ID().String()).Int("processed", processed).Int("skipped", skipped).
		Msg("RM cost V2 job completed")
	return nil
}

func (h *ExecuteHandlerV2) failJob(ctx context.Context, exec *job.Execution, calcErr error) error {
	if failErr := exec.Fail(calcErr.Error()); failErr != nil {
		return fmt.Errorf("fail job: %w (original: %w)", failErr, calcErr)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to failed: %w (original: %w)", err, calcErr)
	}
	h.logger.Error().Err(calcErr).Str("job_id", exec.ID().String()).Msg("RM cost V2 job failed")
	// Suppress the calc error so the consumer doesn't requeue/DLQ — DB has the fail row.
	_ = rmcost.ErrEmptyCalculatedBy // keep import live (unused otherwise in test paths)
	return calcErr
}
