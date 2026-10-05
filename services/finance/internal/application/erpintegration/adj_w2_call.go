package erpintegration

// adj_w2_call.go issues one W2 package call (PKG_GOAPPS_ADJ.VALUATE_ADJ /
// APPROVE_ADJ / RESTORE_ADJ / LOCK_BATCH) through the OracleWriter port only
// (plan-06 P5-T5; design Part 2 §9.3, §9.4; Part 1 §S-R11). Every attempt is
// one STARTED row (committed before the call) plus one terminal row in the
// call log (AC-08). ORA-00054 / ORA-30006 are retried a bounded number of
// times with exponential backoff (2s, 4s, 8s); nothing else is retried.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// alreadyDonePrefix starts the P_SUMMARY of an idempotent no-op call (G9).
const alreadyDonePrefix = "ALREADY_DONE"

// defaultW2Backoff is the first busy backoff; it doubles per attempt.
const defaultW2Backoff = 2 * time.Second

// w2Call is one logical W2 call.
type w2Call struct {
	key     string
	batchID int64
	jobID   string
	actor   string
	params  any
	fn      func(ctx context.Context, w domain.OracleWriter) (domain.Summary, error)
}

// w2Outcome is the result of the last attempt.
type w2Outcome struct {
	summary  domain.Summary
	fin      domain.OracleCallFinish
	attempts int
	callID   string
	writeErr error
}

// alreadyDone reports a successful idempotent no-op (G9: counts as success).
func (o w2Outcome) alreadyDone() bool {
	return o.writeErr == nil && strings.HasPrefix(strings.TrimSpace(o.summary.Text), alreadyDonePrefix)
}

// w2Caller runs W2 calls with the call log, the call timeout and the busy
// retry.
type w2Caller struct {
	writer    WriterGate
	calls     domain.OracleCallLog
	timeout   time.Duration
	retries   int
	backoff   time.Duration
	sleep     func(ctx context.Context, d time.Duration) error
	newCallID func() string
}

func newW2Caller(writer WriterGate, calls domain.OracleCallLog, timeout time.Duration, retries int) *w2Caller {
	if retries < 0 {
		retries = 0
	}
	return &w2Caller{
		writer: writer, calls: calls, timeout: timeout, retries: retries,
		backoff: defaultW2Backoff, sleep: sleepCtx, newCallID: uuid.NewString,
	}
}

// run issues the call, retrying a busy outcome up to c.retries times. ctx
// cancellation is honored before an attempt starts and while waiting; an
// issued Oracle call is never cancelled (only its timeout bounds it). err
// (not out.writeErr) means the first attempt was never issued.
func (c *w2Caller) run(ctx context.Context, call w2Call) (w2Outcome, error) {
	params, err := json.Marshal(call.params)
	if err != nil {
		return w2Outcome{}, fmt.Errorf("encode %s call params: %w", call.key, err)
	}
	var out w2Outcome
	for attempt := 1; ; attempt++ {
		if err := c.once(ctx, call, params, attempt, &out); err != nil {
			if attempt == 1 {
				return w2Outcome{}, err
			}
			// A retry could not start: the last (busy) outcome stands.
			log.Warn().Err(err).Str("key", call.key).Int64("batch_id", call.batchID).Msg("erp w2: retry not started")
			return out, nil
		}
		if !isBusy(out.writeErr) || attempt > c.retries {
			return out, nil
		}
		if !c.wait(ctx, c.backoff<<(attempt-1)) {
			// Cancelled while waiting: the last (busy) outcome stands.
			return out, nil
		}
	}
}

// wait sleeps d before a busy retry; false means ctx ended first.
func (c *w2Caller) wait(ctx context.Context, d time.Duration) bool {
	return c.sleep(ctx, d) == nil
}

func (c *w2Caller) once(ctx context.Context, call w2Call, params []byte, attempt int, out *w2Outcome) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s cancelled before the oracle call: %w", call.key, err)
	}
	callID := c.newCallID()
	if err := c.calls.Start(ctx, domain.OracleCallStart{
		CallID: callID, BatchID: call.batchID, JobID: call.jobID, StatementKey: call.key,
		Params: params, Actor: strings.TrimSpace(call.actor), Attempt: attempt,
	}); err != nil {
		return fmt.Errorf("log %s call start: %w", call.key, err)
	}
	callCtx, cancel := c.callContext(ctx)
	sum, werr := call.fn(callCtx, c.writer.Writer)
	cancel()
	fin := classifyW2(sum, werr)
	if err := c.calls.Finish(context.WithoutCancel(ctx), callID, fin); err != nil {
		log.Error().Err(err).Str("call_id", callID).Int64("batch_id", call.batchID).Str("key", call.key).
			Msg("erp w2: log call finish failed")
	}
	*out = w2Outcome{summary: sum, fin: fin, attempts: attempt, callID: callID, writeErr: werr}
	return nil
}

// callContext detaches the call from job cancellation (cancel is honored
// only before the call) and bounds it with the call timeout.
func (c *w2Caller) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(ctx)
	if c.timeout > 0 {
		return context.WithTimeout(base, c.timeout)
	}
	return context.WithCancel(base)
}

// classifyW2 maps a W2 outcome to the call-log terminal status. A timeout or
// an unclassified error is UNKNOWN (resolved by the read-only probe).
func classifyW2(sum domain.Summary, err error) domain.OracleCallFinish {
	fin := classifyWrite(domain.WriteResult{}, err)
	if err == nil {
		fin.Rows = nil
	}
	if errors.Is(err, domain.ErrOracleTimeout) {
		fin.Status = domain.OracleCallUnknown
	}
	if t := strings.TrimSpace(sum.Text); t != "" {
		if raw, merr := json.Marshal(map[string]string{"text": truncateText(t)}); merr == nil {
			fin.Summary = raw
		}
	}
	return fin
}

// isBusy reports ORA-00054 / ORA-30006 (resource busy).
func isBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrOracleBusy) {
		return true
	}
	var app *domain.OracleAppError
	return errors.As(err, &app) && (app.Code == domain.OraResourceBusy || app.Code == domain.OraResourceBusyWait)
}

// oraCodeIs reports an Oracle application error with code.
func oraCodeIs(err error, code int) bool {
	var app *domain.OracleAppError
	return errors.As(err, &app) && app.Code == code
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
