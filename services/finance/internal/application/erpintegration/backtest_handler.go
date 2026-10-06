package erpintegration

// backtest_handler.go runs the historical SHADOW backtest (plan P5-T10a;
// design Part 2 §10.1, user decision U-4): a SHADOW batch is derived and
// validated for a past period and compared with the legacy ADJ rates. The
// handler has no writer at all, so it can never touch Oracle other than the
// SELECT-only LegacyAdjRateReader.

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErrBacktestPeriodRefused is returned for a period outside the backtest set
// (202605 is excluded until E-11).
var ErrBacktestPeriodRefused = fmt.Errorf("%w: backtest period not allowed", domain.ErrInvalidBatchPeriod)

// backtestPeriods is the allowed backtest period set.
var backtestPeriods = map[string]bool{
	"202604": true, "202606": true, "202607": true, "202608": true, "202609": true,
}

// BacktestCommand requests a backtest of a historical period.
type BacktestCommand struct {
	Period string
	Actor  string
}

type backtestStepFn func(ctx context.Context, batchID int64, actor string) error

// BacktestHandler orchestrates the SHADOW batch and the comparison.
type BacktestHandler struct {
	create func(ctx context.Context, cmd CreateBatchCommand) (*domain.Batch, error)
	steps  []backtestStepFn
	lister StdRowLister
	reader domain.LegacyAdjRateReader
}

// NewBacktestHandler builds the handler. There is deliberately no writer
// parameter.
func NewBacktestHandler(create *CreateBatchHandler, load *LoadDemandStep, cov *CoverageStep, derive *DeriveStep,
	validate *ValidateStep, lister StdRowLister, reader domain.LegacyAdjRateReader,
) *BacktestHandler {
	return &BacktestHandler{
		create: create.Handle,
		steps: []backtestStepFn{
			func(ctx context.Context, id int64, a string) error {
				if load == nil {
					return errors.New("load demand step not configured")
				}
				_, err := load.Run(ctx, id, a, nil)
				return err
			},
			func(ctx context.Context, id int64, a string) error { _, err := cov.Run(ctx, id, a, nil); return err },
			func(ctx context.Context, id int64, a string) error { _, err := derive.Run(ctx, id, a, nil); return err },
			func(ctx context.Context, id int64, a string) error {
				_, err := validate.Run(ctx, id, a, nil)
				return err
			},
		},
		lister: lister, reader: reader,
	}
}

// Run executes the backtest and returns the report and the SHADOW batch id.
// On a step error the batch id is still returned for diagnosis.
func (h *BacktestHandler) Run(ctx context.Context, cmd BacktestCommand) (domain.BacktestReport, int64, error) {
	if !backtestPeriods[cmd.Period] {
		return domain.BacktestReport{}, 0, fmt.Errorf("%w: %q", ErrBacktestPeriodRefused, cmd.Period)
	}
	if h == nil || h.create == nil || h.lister == nil || h.reader == nil {
		return domain.BacktestReport{}, 0, errors.New("erpintegration: backtest handler not configured")
	}
	b, err := h.create(ctx, CreateBatchCommand{Period: cmd.Period, Mode: domain.ModeShadow, Actor: cmd.Actor})
	if err != nil {
		return domain.BacktestReport{}, 0, fmt.Errorf("backtest: create shadow batch: %w", err)
	}
	id := b.ID()
	names := []string{"load demand", "coverage", "derive", "validate"}
	for i, step := range h.steps {
		if err := step(ctx, id, cmd.Actor); err != nil {
			return domain.BacktestReport{}, id, fmt.Errorf("backtest: %s: %w", names[i], err)
		}
	}
	std, err := h.lister.List(ctx, id)
	if err != nil {
		return domain.BacktestReport{}, id, fmt.Errorf("backtest: list std rows: %w", err)
	}
	legacy, err := h.reader.ReadLegacyAdjRates(ctx, cmd.Period)
	if err != nil {
		return domain.BacktestReport{}, id, fmt.Errorf("backtest: read legacy rates: %w", err)
	}
	return domain.CompareBacktest(std, legacy), id, nil
}
