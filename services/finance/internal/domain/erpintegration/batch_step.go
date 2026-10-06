package erpintegration

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Step-support errors (plan-04 P3-T4).
var (
	// ErrDemandStale is returned when a step after LoadDemand runs on demand
	// older than erp_integration.demand_max_age (V-10 freshness, Q11). The
	// operator reloads the demand.
	ErrDemandStale = errors.New("erpintegration: demand snapshot is stale; reload the demand")
	// ErrDemandNotLoaded is returned when a step needs demand that the batch
	// has never loaded.
	ErrDemandNotLoaded = errors.New("erpintegration: demand not loaded")
)

// CheckDemandFresh enforces the V-10 freshness half for every step after
// LoadDemand: now - demand_loaded_at must be <= maxAge. A batch that has
// never loaded demand returns ErrDemandNotLoaded. maxAge <= 0 disables the
// age check (tests / explicit config), never the loaded check.
func CheckDemandFresh(b *Batch, now time.Time, maxAge time.Duration) error {
	if b == nil || b.demandLoadedAt == nil {
		return ErrDemandNotLoaded
	}
	if maxAge <= 0 {
		return nil
	}
	age := now.Sub(*b.demandLoadedAt)
	if age > maxAge {
		return fmt.Errorf("%w: loaded %s ago (max %s)", ErrDemandStale, age.Truncate(time.Second), maxAge)
	}
	return nil
}

// ReopenCoverage moves a COVERED batch back to DEMAND_LOADED because a
// re-coverage found blocking gaps (design §5.3: COVERED only with 0 blocking
// gaps). Unlike a reload it keeps the demand rows and demand_loaded_at (the
// demand did not change, so V-10 freshness must not be reset); coverage and
// everything downstream is invalidated. Only COVERED is accepted.
func (b *Batch) ReopenCoverage(actor string, at time.Time) (Invalidation, error) {
	a, err := normalizeActor(actor)
	if err != nil {
		return Invalidation{}, err
	}
	if b.status != StatusCovered {
		return Invalidation{}, fmt.Errorf("%w: reopen coverage in %s", ErrInvalidTransition, b.status)
	}
	inv := Invalidation{Coverage: true, StdRows: true, RuleSnapshot: true, WarningsAck: true}
	b.ruleSnapshot, b.ruleHash, b.totals = nil, "", nil
	b.warningsAck = nil
	b.status = StatusDemandLoaded
	b.touch(a, at)
	return inv, nil
}

// SetErrorText records the last step error (ceib_error) without changing the
// status: a failed step job sets job_execution FAILED + ceib_error, and the
// batch status changes only for fatal cases (design §10). Empty clears it.
func (b *Batch) SetErrorText(text string) { b.errText = strings.TrimSpace(text) }
