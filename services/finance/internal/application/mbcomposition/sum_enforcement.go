package mbcomposition

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// sumEnforcedEnv is the feature flag that turns the composition-sum rule ([G.5],
// decision D17) from advisory into blocking. Default OFF: production still holds
// 4 legacy recipes whose percentages do not add up, and reading/exporting them
// must keep working until the user has fixed and recalculated them.
const sumEnforcedEnv = "MB_COMPOSITION_SUM_ENFORCED"

// sumEnforced reports whether the composition-sum rule blocks writes.
// Read per call (not cached) so it can be flipped without a rebuild and so tests
// can toggle it with t.Setenv.
func sumEnforced() bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(sumEnforcedEnv)))
	if err != nil {
		return false
	}
	return v
}

// parsePct parses a decimal percentage string; an empty string is 0.
func parsePct(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("mbcomposition: parse percentage %q: %w", s, err)
	}
	return f, nil
}

// sumGuardFor builds the mbcomposition.SumGuard that validates the total mbhID
// would have *after* the pending write, where delta is the signed change this write
// applies to the sum of non-carrier percentages.
//
// It returns nil when the feature flag is off, which the repository reads as "no
// check": no parent-row lock is taken and no sum is read, so the unenforced path is
// unchanged in cost and behavior.
//
// ⭐ G24 (2026-08-22) — this closes the read-then-write race the previous
// enforceSum documented as a known gap. The sum is no longer read on a separate
// connection ahead of the write; the guard is handed to the repository, which
// reads it inside the same transaction as the write and under a FOR UPDATE lock on
// the parent mst_mb_head row. Two concurrent writers on one mbh_id are therefore
// serialized: the second one sees the first one's committed total, not the stale
// pre-write total, so "both read 90, both add 10, both pass" can no longer happen.
func sumGuardFor(delta float64) mbcomposition.SumGuard {
	if !sumEnforced() {
		return nil
	}
	return func(currentSum string) error {
		total, err := parsePct(currentSum)
		if err != nil {
			return err
		}
		// rowCount is at least 1: the row being created or updated by this very call
		// is part of the composition, so the empty-composition case cannot apply here.
		return mbcomposition.ValidateSum(total+delta, 1)
	}
}

// pctDelta returns how much a row contributes to the non-carrier percentage sum:
// its parsed percentage, or 0 when it is a carrier row (carrier rows are excluded
// by SumPercentageByMbhID).
func pctDelta(pct string, isCarrier bool) (float64, error) {
	if isCarrier {
		return 0, nil
	}
	return parsePct(pct)
}

// EnforceHeadSum checks the composition-sum rule against a head's CURRENT stored
// composition, for the workflow gates that do not write composition rows
// themselves — submit and validate (plan §11 item 78, the remainder of [G.5]).
//
// Exported because those gates live in the mbhead application package. It is a
// no-op when MB_COMPOSITION_SUM_ENFORCED is off, which is what keeps the 4 legacy
// recipes with broken totals submittable today.
//
// ⚠ Unlike the create/update paths this reads the composition WITHOUT a lock, and
// deliberately so: submit/validate are read-then-transition operations whose write
// touches mst_mb_head, not mst_mb_composition, so there is no composition write to
// make atomic here. The residual window is "someone edits the composition while a
// submit is in flight" — and that edit is itself guarded by CreateWithSumGuard /
// UpdateWithSumGuard, so it cannot leave a bad total behind either way.
//
// rowCount is the real number of composition rows, so an empty composition reports
// ErrCompositionEmpty rather than a misleading "does not total 100%" (R17).
func EnforceHeadSum(ctx context.Context, repo mbcomposition.Repository, mbhID string) error {
	if !sumEnforced() {
		return nil
	}
	rows, err := repo.ListByMbhID(ctx, mbhID)
	if err != nil {
		return err
	}

	var total float64
	for _, row := range rows {
		d, pErr := pctDelta(row.CompositionPct(), row.IsCarrier())
		if pErr != nil {
			return pErr
		}
		total += d
	}
	return mbcomposition.ValidateSum(total, len(rows))
}
