// Package oraclesync provides application-level orchestration for Oracle-to-PostgreSQL sync jobs.
package oraclesync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

const (
	// OracleSchema is the Oracle schema containing the stored procedure.
	OracleSchema = "MGTAPPS"
	// OracleProcedure is the stored procedure that refreshes Oracle data.
	OracleProcedure = "SP_REFRESH_ITEM_CONS_STK_PO"

	stepProcedure = "execute_procedure"
	stepFetch     = "fetch_data"
	stepUpsert    = "upsert_data"
)

// SyncHandler orchestrates the Oracle sync process.
type SyncHandler struct {
	jobRepo    job.Repository
	oracleRepo syncdata.OracleSourceRepository
	pgRepo     syncdata.PostgresTargetRepository
	logger     zerolog.Logger
}

// NewSyncHandler creates a new SyncHandler.
func NewSyncHandler(
	jobRepo job.Repository,
	oracleRepo syncdata.OracleSourceRepository,
	pgRepo syncdata.PostgresTargetRepository,
	logger zerolog.Logger,
) *SyncHandler {
	return &SyncHandler{
		jobRepo:    jobRepo,
		oracleRepo: oracleRepo,
		pgRepo:     pgRepo,
		logger:     logger,
	}
}

// Execute runs the full sync workflow for a given job.
func (h *SyncHandler) Execute(ctx context.Context, jobID uuid.UUID) error {
	// Fetch the job execution.
	exec, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", jobID, err)
	}

	// Transition to processing.
	if err := exec.Start(); err != nil {
		return fmt.Errorf("start job %s: %w", jobID, err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to processing: %w", err)
	}

	h.logger.Info().
		Str("job_id", jobID.String()).
		Str("period", exec.Period()).
		Msg("Starting Oracle sync job")

	// Run the sync steps. On any failure, mark the job as failed.
	if syncErr := h.runSync(ctx, exec); syncErr != nil {
		h.logger.Error().Err(syncErr).
			Str("job_id", jobID.String()).
			Msg("Sync job failed")
		return h.failJob(ctx, exec, syncErr)
	}

	return nil
}

func (h *SyncHandler) runSync(ctx context.Context, exec *job.Execution) error {
	period := exec.Period()
	jobID := exec.ID()

	// Step 1: Execute Oracle stored procedure.
	if err := h.executeProcedure(ctx, jobID, period); err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 30); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 30%")
	}

	// Step 2: Fetch data from Oracle.
	items, err := h.fetchData(ctx, jobID, period)
	if err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 60); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 60%")
	}

	// Step 3: Upsert to PostgreSQL.
	result, err := h.upsertData(ctx, jobID, items)
	if err != nil {
		return err
	}
	if err := h.jobRepo.UpdateProgress(ctx, jobID, 100); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to update progress to 100%")
	}

	// Complete the job with result summary.
	return h.completeJob(ctx, exec, result)
}

func (h *SyncHandler) executeProcedure(ctx context.Context, jobID uuid.UUID, period string) error {
	logEntry := job.NewExecutionLog(jobID, stepProcedure, job.LogStarted,
		fmt.Sprintf("Executing %s.%s for period %s", OracleSchema, OracleProcedure, period), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add procedure log")
	}

	start := time.Now()
	err := h.oracleRepo.ExecuteProcedureWithParam(ctx, OracleSchema, OracleProcedure, period)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Procedure failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update procedure failure log")
		}
		return fmt.Errorf("%w: %v", syncdata.ErrProcedureFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess, fmt.Sprintf("Procedure completed in %s", duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update procedure success log")
	}

	h.logger.Info().
		Dur("duration", duration).
		Str("period", period).
		Msg("Oracle procedure completed")

	return nil
}

func (h *SyncHandler) fetchData(ctx context.Context, jobID uuid.UUID, period string) ([]*syncdata.ItemConsStockPO, error) {
	logEntry := job.NewExecutionLog(jobID, stepFetch, job.LogStarted,
		fmt.Sprintf("Fetching data for period %s from Oracle", period), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add fetch log")
	}

	start := time.Now()
	items, err := h.oracleRepo.FetchItemConsStockPO(ctx, period)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Fetch failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update fetch failure log")
		}
		return nil, fmt.Errorf("%w: %v", syncdata.ErrFetchFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess, fmt.Sprintf("Fetched %d rows in %s", len(items), duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update fetch success log")
	}

	h.logger.Info().
		Int("rows", len(items)).
		Dur("duration", duration).
		Str("period", period).
		Msg("Oracle data fetch completed")

	return items, nil
}

func (h *SyncHandler) upsertData(ctx context.Context, jobID uuid.UUID, items []*syncdata.ItemConsStockPO) (*syncdata.UpsertResult, error) {
	logEntry := job.NewExecutionLog(jobID, stepUpsert, job.LogStarted,
		fmt.Sprintf("Upserting %d rows to PostgreSQL", len(items)), nil)
	if err := h.jobRepo.AddLog(ctx, logEntry); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to add upsert log")
	}

	start := time.Now()
	result, err := h.pgRepo.UpsertItemConsStockPO(ctx, items, jobID)
	duration := time.Since(start)

	if err != nil {
		logEntry.MarkCompleted(job.LogFailed, fmt.Sprintf("Upsert failed after %s: %v", duration, err))
		if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
			h.logger.Warn().Err(logErr).Msg("Failed to update upsert failure log")
		}
		return nil, fmt.Errorf("%w: %v", syncdata.ErrUpsertFailed, err)
	}

	logEntry.MarkCompleted(job.LogSuccess,
		fmt.Sprintf("Upserted %d rows (%d inserted, %d updated) in %s",
			result.TotalRows, result.Inserted, result.Updated, duration))
	if logErr := h.jobRepo.UpdateLog(ctx, logEntry); logErr != nil {
		h.logger.Warn().Err(logErr).Msg("Failed to update upsert success log")
	}

	h.logger.Info().
		Int("total", result.TotalRows).
		Int("inserted", result.Inserted).
		Int("updated", result.Updated).
		Dur("duration", duration).
		Msg("PostgreSQL upsert completed")

	return result, nil
}

func (h *SyncHandler) completeJob(ctx context.Context, exec *job.Execution, result *syncdata.UpsertResult) error {
	summary, err := json.Marshal(map[string]any{
		"total_rows": result.TotalRows,
		"inserted":   result.Inserted,
		"updated":    result.Updated,
		"period":     exec.Period(),
	})
	if err != nil {
		return fmt.Errorf("marshal result summary: %w", err)
	}

	if err := exec.Complete(summary); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("update status to success: %w", err)
	}

	h.logger.Info().
		Str("job_id", exec.ID().String()).
		Str("code", exec.Code().String()).
		Msg("Oracle sync job completed successfully")

	return nil
}

func (h *SyncHandler) failJob(ctx context.Context, exec *job.Execution, syncErr error) error {
	if failErr := exec.Fail(syncErr.Error()); failErr != nil {
		h.logger.Error().Err(failErr).Msg("Failed to transition job to failed state")
		return fmt.Errorf("fail job: %w (original: %v)", failErr, syncErr)
	}
	if err := h.jobRepo.UpdateStatus(ctx, exec); err != nil {
		h.logger.Error().Err(err).Msg("Failed to persist job failure status")
		return fmt.Errorf("update status to failed: %w (original: %v)", err, syncErr)
	}
	return syncErr
}

// ResolvePeriod returns the sync period based on the current date.
// Day 1-3: previous month, Day 4+: current month.
// Format: YYYYMM (e.g., "202601").
func ResolvePeriod(now time.Time) string {
	if now.Day() <= 3 {
		prev := now.AddDate(0, -1, 0)
		return prev.Format("200601")
	}
	return now.Format("200601")
}
