package erpintegration

// gates.go holds the reusable safety gates of the Oracle-writing steps
// (plan-06 P5-T3 step 1; design Part 1 §3.2, AC-06). Each gate fails closed
// and is checked before any Oracle writer call:
//
//   - G1  feature flag (erp_integration.push_enabled / valuation_enabled …)
//   - G2  writer configured and not disabled (writer_mode)
//   - G3  permission (RBAC outcome passed in from delivery)
//   - G10 (period, ACTUAL) locked, checked inside the G11 transaction
//   - G11 per-batch advisory lock (BatchTxRunner.RunLocked; its refusal is
//     domain.ErrConcurrentRun)

import (
	"context"
	"errors"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErrPushPermissionDenied is returned when the caller lacks
// finance.cost.erpintegration.push (G3).
var ErrPushPermissionDenied = errors.New("erpintegration: permission denied to push (finance.cost.erpintegration.push)")

// WriterGate is the writer half of the gates: the writer and its effective
// mode as returned by the fail-closed factory.
type WriterGate struct {
	Writer domain.OracleWriter
	Mode   domain.WriterMode
}

// checkFlag is G1: the named feature flag must be on.
func checkFlag(enabled bool, flag string) error {
	if !enabled {
		return fmt.Errorf("%w: %s is off", domain.ErrFeatureDisabled, flag)
	}
	return nil
}

// checkWriter is G2: a writer must be wired and its mode a known,
// non-disabled mode.
func checkWriter(w WriterGate) error {
	if w.Writer == nil || !w.Mode.IsValid() || w.Mode == domain.WriterModeDisabled {
		return fmt.Errorf("%w: writer_mode %q", domain.ErrWriterNotConfigured, w.Mode)
	}
	return nil
}

// checkPermission is G3: the delivery-layer RBAC outcome.
func checkPermission(has bool, denied error) error {
	if !has {
		return denied
	}
	return nil
}

// periodLockReader is the G10 part of a tx-scoped store.
type periodLockReader interface {
	IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error)
}

// checkPeriodLocked is G10: (period, ACTUAL) must be locked. Called inside
// the G11 transaction so the FOR SHARE read holds until commit.
func checkPeriodLocked(ctx context.Context, st periodLockReader, period string) error {
	locked, err := st.IsPeriodLocked(ctx, period, periodLockCalcType)
	if err != nil {
		return fmt.Errorf("check period lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("%w: %s %s", domain.ErrPeriodNotLocked, period, periodLockCalcType)
	}
	return nil
}

// checkPushBatch checks the batch itself may be pushed: LIVE (SHADOW never
// reaches the writer), VALIDATED, never pushed, not flagged needs-repush,
// with control totals and rule hash, and validated for its current
// derivation.
func checkPushBatch(b *domain.Batch) error {
	if b.Mode() == domain.ModeShadow {
		return domain.ErrShadowNotPushable
	}
	if b.Status() != domain.StatusValidated {
		return fmt.Errorf("%w: push in %s", ErrStepNotAllowed, b.Status())
	}
	if b.IsFrozen() {
		return domain.ErrBatchFrozen
	}
	if b.NeedsRepush() {
		return domain.ErrNeedsRepush
	}
	if b.RuleHash() == "" || b.Totals() == nil {
		return domain.ErrControlTotalsMissing
	}
	var v ValidateSummary
	if !readSummaryEntry(b.Summary(), StepValidate, &v) || !v.Validated {
		return fmt.Errorf("%w: batch not validated", domain.ErrValidationFailed)
	}
	if v.RuleHash != b.RuleHash() || !sameTime(v.DeriveRunAt, summaryDeriveRunAt(b.Summary())) {
		return fmt.Errorf("%w: batch re-derived since the last validate", domain.ErrValidationFailed)
	}
	return nil
}
