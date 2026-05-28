// Package job provides application-layer handlers for BI ETL job listing + manual triggers.
//
// In MVP the Trigger handler records a placeholder log entry (RUNNING → SUCCESS) without
// actually executing the Oracle procedure. Spec 1D wires the real RabbitMQ → finance-bi-worker
// bridge that turns this trigger into a real job run.
package job

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// ListHandler returns the job registry with last-run summary.
type ListHandler struct{ repo jobdomain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r jobdomain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle returns jobs.
func (h *ListHandler) Handle(ctx context.Context, includeInactive bool) ([]*jobdomain.Job, error) {
	return h.repo.List(ctx, includeInactive)
}

// ListLogsQuery is the payload for ListLogsHandler.
type ListLogsQuery struct {
	JobID    uuid.UUID
	Page     int
	PageSize int
}

// ListLogsResult bundles the page + total count.
type ListLogsResult struct {
	Items []*jobdomain.Log
	Total int64
}

// ListLogsHandler returns paginated logs for a single job.
type ListLogsHandler struct{ repo jobdomain.Repository }

// NewListLogsHandler constructs a ListLogsHandler.
func NewListLogsHandler(r jobdomain.Repository) *ListLogsHandler { return &ListLogsHandler{repo: r} }

// Handle executes the list-logs query.
func (h *ListLogsHandler) Handle(ctx context.Context, q ListLogsQuery) (ListLogsResult, error) {
	items, total, err := h.repo.ListLogs(ctx, q.JobID, q.Page, q.PageSize)
	if err != nil {
		return ListLogsResult{}, fmt.Errorf("list job logs: %w", err)
	}
	return ListLogsResult{Items: items, Total: total}, nil
}

// TriggerCommand is the payload for TriggerHandler.
type TriggerCommand struct {
	JobID       uuid.UUID
	TriggeredBy uuid.UUID
}

// TriggerHandler records a manual job trigger.
//
// In MVP (spec 1A+1B): inserts a RUNNING log row, then immediately marks it SUCCESS with
// rows_affected=0 — a placeholder that lets the admin UI verify the trigger button works
// without actually running Oracle procedures. Spec 1D replaces this body with a RabbitMQ
// publish to the finance-bi-worker queue.
type TriggerHandler struct{ repo jobdomain.Repository }

// NewTriggerHandler constructs a TriggerHandler.
func NewTriggerHandler(r jobdomain.Repository) *TriggerHandler { return &TriggerHandler{repo: r} }

// Handle executes the manual trigger (placeholder MVP behavior).
func (h *TriggerHandler) Handle(ctx context.Context, cmd TriggerCommand) (*jobdomain.Log, error) {
	if _, err := h.repo.GetByID(ctx, cmd.JobID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	log := &jobdomain.Log{
		JobID:       cmd.JobID,
		StartedAt:   now,
		Status:      jobdomain.StatusRunning,
		TriggeredBy: "MANUAL:" + cmd.TriggeredBy.String(),
	}
	if err := h.repo.InsertLog(ctx, log); err != nil {
		return nil, fmt.Errorf("insert running log: %w", err)
	}

	// MVP: immediately resolve to SUCCESS placeholder. Real work happens in spec 1D.
	ended := time.Now().UTC()
	log.EndedAt = ended
	log.Status = jobdomain.StatusSuccess
	log.DurationMs = int(ended.Sub(now).Milliseconds())
	if err := h.repo.UpdateLog(ctx, log); err != nil {
		return nil, fmt.Errorf("update completion log: %w", err)
	}
	return log, nil
}
