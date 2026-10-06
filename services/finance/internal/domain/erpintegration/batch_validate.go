package erpintegration

// batch_validate.go holds the Validate-step support of the Batch aggregate
// (plan-06 P5-T2; design Part 1 §5.3, Part 2 §7).

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// RevokeValidation moves a VALIDATED batch back to DERIVED because a
// re-validation no longer passes (an error appeared, the warning set changed
// so the ack no longer matches, or demand went stale). Unlike a re-derive
// (Transition to DERIVED) nothing is invalidated: the std rows, rule
// snapshot, hash and control totals still belong to the current derivation,
// and the warnings ack stamp is kept (the validate step binds it to the
// warning-set hash, so a changed set is never treated as acked). A frozen
// (pushed) batch is never revoked.
func (b *Batch) RevokeValidation(actor string, at time.Time) error {
	a, err := normalizeActor(actor)
	if err != nil {
		return err
	}
	if b.IsFrozen() {
		return ErrBatchFrozen
	}
	if b.status != StatusValidated {
		return fmt.Errorf("%w: revoke validation in %s", ErrInvalidTransition, b.status)
	}
	b.status = StatusDerived
	b.touch(a, at)
	return nil
}

// ValidationBaseline is the V-05 / rule-diff baseline of a batch: the latest
// LIVE VALUATED / RECONCILED / LOCKED batch of the same or an earlier period
// (the "previous active batch", design §7 V-05, Part 1 §4.2
// cesc_prev_std_cost). Std holds its OK rows' std cost per key.
type ValidationBaseline struct {
	BatchID      int64
	Period       string
	RuleSnapshot []byte
	Std          map[ErpKey]decimal.Decimal
}

// CostLabel is the currency label and value of a source cst_product_cost row
// (V-09).
type CostLabel struct {
	Currency    string
	CostPerUnit decimal.Decimal
}

// StdValidationUpdate is the cesc_validation / cesc_prev_std_cost write of
// one std row. Derived are the issues Derive attached (kept as they are);
// Validation are the validate-step findings not already present in Derived.
// PrevStd is the V-05 baseline std (NULL for first-seen keys).
type StdValidationUpdate struct {
	Key        ErpKey
	Derived    []Issue
	Validation []Issue
	PrevStd    decimal.NullDecimal
}

// ReplicaCodes are the codes found in the PG ERP master replicas.
type ReplicaCodes struct {
	Items  map[string]struct{}
	Shades map[string]struct{}
	Grades map[string]struct{}
}

// ValidateStore is the transaction-scoped store of the Validate step. The PG
// BatchTxRunner hands a BatchStore that also implements it; every read
// shares the G11-locked transaction.
type ValidateStore interface {
	BatchStore
	// ListCoverage returns the batch's coverage rows ordered by (item, shade).
	ListCoverage(ctx context.Context) ([]CoverageLine, error)
	// IsPeriodLocked is the G10 check inside the transaction (FOR SHARE).
	IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error)
	// ListStdRowsForValidation returns the batch's std rows ordered by key,
	// with Issues holding only the issues Derive attached (the validate-step
	// findings of an earlier run are excluded, so they are recomputed).
	ListStdRowsForValidation(ctx context.Context) ([]StdRow, error)
	// UpdateStdValidation writes cesc_validation and cesc_prev_std_cost of
	// the given rows (matched by key within the batch). It returns the number
	// of rows updated.
	UpdateStdValidation(ctx context.Context, updates []StdValidationUpdate) (int64, error)
	// ReplicaCodes returns which of the given trimmed codes exist in
	// cost_erp_item, cost_erp_shade and cost_erp_grade.
	ReplicaCodes(ctx context.Context, items, shades, grades []string) (ReplicaCodes, error)
	// SourceCostLabels returns the currency label and cost per unit of the
	// given cst_product_cost ids; unknown ids are absent.
	SourceCostLabels(ctx context.Context, costIDs []int64) (map[int64]CostLabel, error)
	// CurrencyRelabelApplied reports whether migration 000557 relabelled
	// period (cst_currency_relabel_log rows for the period).
	CurrencyRelabelApplied(ctx context.Context, period string) (bool, error)
	// FindValidationBaseline returns the baseline of the batch (see
	// ValidationBaseline), or nil when there is none.
	FindValidationBaseline(ctx context.Context, period string) (*ValidationBaseline, error)
}
