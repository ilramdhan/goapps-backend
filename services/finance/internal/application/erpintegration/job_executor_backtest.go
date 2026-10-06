package erpintegration

// job_executor_backtest.go routes the erp_integration backtest job (plan
// P5-T10b, design Part 2 §10.1) to the BacktestHandler: a SHADOW derivation
// of a historical period compared with the legacy ADJ rates. Like attr_backfill
// it carries no batch_id; params are {period?} (default: the job period).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// SubtypeBacktest is the erp_integration job subtype of the backtest.
const SubtypeBacktest = "backtest"

// BacktestJobParams is the backtest job params payload.
type BacktestJobParams struct {
	Period string `json:"period,omitempty"`
}

// BacktestJobResult is the backtest job result summary.
type BacktestJobResult struct {
	BatchID int64          `json:"batch_id"`
	Period  string         `json:"period"`
	Counts  map[string]int `json:"counts"`
	Failed  bool           `json:"failed"`
}

// ParseBacktestJobParams decodes the params; empty params are valid.
func ParseBacktestJobParams(raw json.RawMessage) (BacktestJobParams, error) {
	var p BacktestJobParams
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return p, nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%w: %w", ErrInvalidJobParams, err)
	}
	p.Period = strings.TrimSpace(p.Period)
	return p, nil
}

// WithBacktest attaches the backtest handler and the report repository.
func (e *JobExecutor) WithBacktest(h *BacktestHandler, repo domain.BacktestReportRepository) *JobExecutor {
	e.backtest, e.backtestRepo = h, repo
	return e
}

// runBacktest runs the backtest job. A failed comparison (SP* differs) is a
// finding: the job still completes with failed:true in the result.
func (e *JobExecutor) runBacktest(ctx context.Context, exec *job.Execution, progress ProgressFunc) error {
	p, err := ParseBacktestJobParams(exec.Params())
	if err != nil {
		return e.failJob(ctx, exec, 0, err)
	}
	if e.backtest == nil || e.backtestRepo == nil {
		return e.failJob(ctx, exec, 0, errors.New("erpintegration: backtest not configured"))
	}
	period := p.Period
	if period == "" {
		period = strings.TrimSpace(exec.Period())
	}
	progress.report(ctx, progressCompute)
	rep, batchID, err := e.backtest.Run(ctx, BacktestCommand{Period: period, Actor: exec.CreatedBy()})
	if err != nil {
		return e.failJob(ctx, exec, batchID, err)
	}
	progress.report(ctx, progressPersist)
	if err := e.backtestRepo.Replace(ctx, batchID, period, rep); err != nil {
		return e.failJob(ctx, exec, batchID, fmt.Errorf("persist backtest report: %w", err))
	}
	counts := make(map[string]int, len(rep.Counts))
	for c, n := range rep.Counts {
		counts[string(c)] = n
	}
	raw, err := json.Marshal(BacktestJobResult{BatchID: batchID, Period: period, Counts: counts, Failed: rep.Failed})
	if err != nil {
		return e.failJob(ctx, exec, batchID, fmt.Errorf("encode result: %w", err))
	}
	if err := exec.Complete(raw); err != nil {
		return fmt.Errorf("complete erp_integration job %s: %w", exec.ID(), err)
	}
	if err := e.jobs.UpdateStatus(ctx, exec); err != nil {
		return fmt.Errorf("mark erp_integration job %s success: %w", exec.ID(), err)
	}
	return nil
}
