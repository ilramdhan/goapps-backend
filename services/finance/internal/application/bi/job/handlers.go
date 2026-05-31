// Package job provides application-layer handlers for BI ETL job listing + manual triggers.
//
// In MVP the Trigger handler records a placeholder log entry (RUNNING → SUCCESS) without
// actually executing the Oracle procedure. Spec 1D wires the real RabbitMQ → finance-bi-worker
// bridge that turns this trigger into a real job run.
package job

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// MVRefresher refreshes BI materialized views.
// Injected into TriggerHandler so the mv_refresh job kind can trigger a view
// refresh without executing an Oracle fetch.
type MVRefresher interface {
	RefreshMVs(ctx context.Context) error
}

// BIETLRunner executes a BI ETL job that loads an Oracle MV into bi_fact_metric.
// A single generic method keeps the interface extensible — adding a new target type
// requires only a new case inside the concrete implementation (MVLoader), not a new
// interface method and not a new switch arm in TriggerHandler.
//
// targetType  — bi_fact_metric.type value (e.g. "MIS", "DELIVERY MARGIN", "SALES").
// sourceView  — fully-qualified Oracle view/MV name (e.g. "MGTDAT.MV_DASH_MIS_MGT").
type BIETLRunner interface {
	Load(ctx context.Context, targetType, sourceView string) (int, error)
}

// TriggerCommand is the payload for TriggerHandler.
type TriggerCommand struct {
	JobID       uuid.UUID
	TriggeredBy uuid.UUID
}

// TriggerHandler records a manual job trigger.
//
// Dispatches by config["kind"]:
//   - "mv_refresh"  — refreshes Postgres materialized views, marks SUCCESS.
//   - "etl_*"       — any kind prefixed "etl_" is dispatched to BIETLRunner.Load
//     using config["target_type"] and config["source_view"].
//     Adding a new ETL type requires only a new case in MVLoader.Load,
//     no changes to TriggerHandler or the admin form.
//   - (other)       — MVP placeholder: immediately marks SUCCESS, rows_affected=0.
type TriggerHandler struct {
	repo        jobdomain.Repository
	mvRefresher MVRefresher // optional — nil when not needed
	etlRunner   BIETLRunner // optional — nil when Oracle not configured
}

// NewTriggerHandler constructs a TriggerHandler.
// etlRunner may be nil (Oracle unavailable); those job kinds will fail gracefully.
func NewTriggerHandler(r jobdomain.Repository, mv MVRefresher, etl BIETLRunner) *TriggerHandler {
	return &TriggerHandler{repo: r, mvRefresher: mv, etlRunner: etl}
}

// Handle executes the manual trigger.
func (h *TriggerHandler) Handle(ctx context.Context, cmd TriggerCommand) (*jobdomain.Log, error) {
	theJob, err := h.repo.GetByID(ctx, cmd.JobID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	entry := &jobdomain.Log{
		JobID:       cmd.JobID,
		StartedAt:   now,
		Status:      jobdomain.StatusRunning,
		TriggeredBy: "MANUAL:" + cmd.TriggeredBy.String(),
	}
	if err = h.repo.InsertLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("insert running log: %w", err)
	}

	kind := ""
	if v, ok := theJob.Config["kind"].(string); ok {
		kind = v
	}

	// Dispatch by job kind.
	switch {
	case kind == "mv_refresh":
		return h.handleMVRefresh(ctx, entry, now)
	case strings.HasPrefix(kind, "etl_"):
		// Generic ETL dispatch: works for any target_type without code changes.
		// target_type and source_view come from the job's config (auto-set on create).
		targetType := configString(theJob.Config, "target_type")
		sourceView := configString(theJob.Config, "source_view")
		return h.handleETL(ctx, entry, now, func(c context.Context) (int, error) {
			return h.etlRunner.Load(c, targetType, sourceView)
		})
	default:
		// MVP placeholder: resolve immediately to SUCCESS without real work.
		ended := time.Now().UTC()
		entry.EndedAt = ended
		entry.Status = jobdomain.StatusSuccess
		entry.DurationMs = int(ended.Sub(now).Milliseconds()) //nolint:gosec // duration in ms always fits int
		if err = h.repo.UpdateLog(ctx, entry); err != nil {
			return nil, fmt.Errorf("update completion log: %w", err)
		}
		return entry, nil
	}
}

// handleETL runs an ETL load function and writes the outcome to the log.
func (h *TriggerHandler) handleETL(
	ctx context.Context,
	entry *jobdomain.Log,
	started time.Time,
	run func(context.Context) (int, error),
) (*jobdomain.Log, error) {
	rowsAffected, runErr := func() (int, error) {
		if h.etlRunner == nil {
			return 0, errors.New("ETL runner not configured (Oracle not connected)")
		}
		return run(ctx)
	}()

	ended := time.Now().UTC()
	entry.EndedAt = ended
	entry.DurationMs = int(ended.Sub(started).Milliseconds()) //nolint:gosec // duration in ms always fits int
	entry.RowsAffected = rowsAffected
	if runErr != nil {
		entry.Status = jobdomain.StatusFailed
		entry.ErrorMessage = runErr.Error()
	} else {
		entry.Status = jobdomain.StatusSuccess
	}
	if err := h.repo.UpdateLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("update ETL completion log: %w", err)
	}
	return entry, nil
}

// handleMVRefresh calls RefreshMVs and writes the result back to the log.
func (h *TriggerHandler) handleMVRefresh(ctx context.Context, entry *jobdomain.Log, started time.Time) (*jobdomain.Log, error) {
	var refreshErr error
	if h.mvRefresher != nil {
		refreshErr = h.mvRefresher.RefreshMVs(ctx)
	}
	ended := time.Now().UTC()
	entry.EndedAt = ended
	entry.DurationMs = int(ended.Sub(started).Milliseconds()) //nolint:gosec // duration in ms always fits int
	if refreshErr != nil {
		entry.Status = jobdomain.StatusFailed
		entry.ErrorMessage = refreshErr.Error()
	} else {
		entry.Status = jobdomain.StatusSuccess
	}
	if err := h.repo.UpdateLog(ctx, entry); err != nil {
		return nil, fmt.Errorf("update mv_refresh completion log: %w", err)
	}
	return entry, nil
}

// configString extracts a string value from a job config map.
// Returns "" when the key is absent or the value is not a string.
// Avoids blank-identifier type assertions that errcheck flags.
func configString(config map[string]any, key string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
