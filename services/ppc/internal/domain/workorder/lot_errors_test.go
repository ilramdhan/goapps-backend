package workorder_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorder "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// renderedLotFailures returns every lot failure a planner can actually receive,
// as the exact string that reaches the wire.
func renderedLotFailures() map[string]string {
	return map[string]string{
		"item/shade":         workorder.NewLotItemShadeError("POY0000451").Error(),
		"std weight":         workorder.NewLotStdWeightError("TTY0000028", "AC3").Error(),
		"no product":         workorder.NewLotProductError().Error(),
		"lot not registered": workorder.ErrLotNotFound.Error(),
		// Reachable at application/workorder/service.go when no lot provisioner
		// is wired. It is a deployment fault rather than missing master data, but
		// it still reaches a planner, so it belongs inside the contract rather
		// than outside it -- left out, it rendered no alert at all.
		"generation unavailable": workorder.ErrLotGenerationUnavailable.Error(),
	}
}

// tsCopyLocation names the unbound duplicate the frozen-literal test points at.
const tsCopyLocation = "goapps-frontend/src/components/ppc/work-order/lot-error-hint.tsx (the CAUSE_* constants)"

// The literal text of every cause phrase is FROZEN here, byte for byte.
//
// This test exists because of a hole the exclusivity test does not close. The
// frontend duplicates these strings as plain TypeScript literals -- no codegen,
// no shared JSON, nothing binding the two languages. A reword applied
// consistently across the Go side keeps exclusivity intact, so
// TestLotCausePhrases_AreMutuallyExclusive still passes; the frontend's own test
// carries its own hardcoded fixture matching the now-stale TS constant, so that
// passes too; and production silently loses the link with every gate green.
//
// Freezing the literals converts that invisible failure into a declared
// two-site edit: rewording forces a change here, and this failure message says
// exactly which frontend file to update in the same commit.
func TestCausePhraseLiterals_AreFrozen(t *testing.T) {
	// Written out as literals on purpose -- comparing a constant to itself would
	// assert nothing.
	want := map[string]string{
		"CausePhraseItemShade":             "no ERP item code and shade code",
		"CausePhraseStdWeight":             "standard bobbin weight (STD_WEIGHT) is not set",
		"CausePhraseNoProduct":             "plan item is not linked to a product",
		"CausePhraseLotNotRegistered":      "lot number is not registered in lot master",
		"CausePhraseGenerationUnavailable": "lot number generation is not available",
	}
	got := map[string]string{
		"CausePhraseItemShade":             workorder.CausePhraseItemShade,
		"CausePhraseStdWeight":             workorder.CausePhraseStdWeight,
		"CausePhraseNoProduct":             workorder.CausePhraseNoProduct,
		"CausePhraseLotNotRegistered":      workorder.CausePhraseLotNotRegistered,
		"CausePhraseGenerationUnavailable": workorder.CausePhraseGenerationUnavailable,
	}

	for name, wantPhrase := range want {
		assert.Equal(t, wantPhrase, got[name],
			"cause phrase %s changed. The frontend keys on this exact string and is NOT "+
				"mechanically bound to it, so you must update both in this change: "+
				"(1) this test's expected literal, and (2) %s. "+
				"Leaving the frontend stale loses the fix-it link in production with every gate green.",
			name, tsCopyLocation)
	}

	// A phrase added to LotCausePhrases but not frozen here would slip the net.
	assert.Len(t, workorder.LotCausePhrases, len(want),
		"every phrase in LotCausePhrases must be frozen above and mirrored in %s", tsCopyLocation)
	for _, phrase := range workorder.LotCausePhrases {
		assert.True(t, slices.Contains(slices.Collect(maps.Values(want)), phrase),
			"phrase %q is in LotCausePhrases but not frozen by this test", phrase)
	}
}

// This is the test that makes the frontend's classifier safe.
//
// BaseResponse carries no structured cause field, so the frontend identifies a
// lot failure by matching a cause phrase in the message text. That is only sound
// if each phrase occurs in EXACTLY ONE rendered message. It was not: the
// "no product" failure's fix hint ends "...or enter a lot number already
// registered in lot master", which a naive `includes("lot master")` matched
// first, linking the planner to Lot Master for the one failure Lot Master cannot
// fix.
//
// Pinning exclusivity here means match ORDER is irrelevant, so a general branch
// can never shadow a specific one, and rewording a phrase breaks this test
// rather than silently breaking a link in the UI.
func TestLotCausePhrases_AreMutuallyExclusive(t *testing.T) {
	rendered := renderedLotFailures()

	for _, phrase := range workorder.LotCausePhrases {
		var matched []string
		for name, msg := range rendered {
			if strings.Contains(msg, phrase) {
				matched = append(matched, name)
			}
		}
		assert.Len(t, matched, 1,
			"cause phrase %q must appear in exactly one rendered message, matched %v — "+
				"the frontend classifier keys on these phrases, so an overlap makes it link to the wrong master",
			phrase, matched)
	}
}

// Every failure must be classifiable: a message matching no phrase would render
// as an alert with no link at all.
func TestEveryLotFailure_MatchesExactlyOneCausePhrase(t *testing.T) {
	for name, msg := range renderedLotFailures() {
		t.Run(name, func(t *testing.T) {
			var matched []string
			for _, phrase := range workorder.LotCausePhrases {
				if strings.Contains(msg, phrase) {
					matched = append(matched, phrase)
				}
			}
			assert.Len(t, matched, 1, "message %q matched %v", msg, matched)
		})
	}
}

// The "no product" cause names the plan item itself, so appending the product
// subject produced "...the plan item is not linked to a product for the selected
// plan item's product". SubjectSuppressed removes that.
func TestNewLotProductError_OmitsTheRedundantProductSubject(t *testing.T) {
	msg := workorder.NewLotProductError().Error()
	assert.Contains(t, msg, workorder.CausePhraseNoProduct)
	assert.NotContains(t, msg, "for the selected plan item's product")
	assert.NotContains(t, msg, " for product ")
	// The actionable half must survive the suppression.
	assert.Contains(t, msg, "Link a product to the plan item")
}

// The subject clause is still present for the causes that need it -- suppressing
// it globally would have lost the product code from the two failures whose whole
// point is naming which product to open.
func TestLabelledCauses_KeepTheProductSubject(t *testing.T) {
	assert.Contains(t, workorder.NewLotItemShadeError("POY0000451").Error(), "for product POY0000451")

	stdWeight := workorder.NewLotStdWeightError("TTY0000028", "AC3").Error()
	assert.Contains(t, stdWeight, "for product TTY0000028")
	assert.Contains(t, stdWeight, "on machine AC3")
}

// A blank machine label must not leave a dangling "on machine " clause.
func TestNewLotStdWeightError_OmitsMachineWhenUnknown(t *testing.T) {
	msg := workorder.NewLotStdWeightError("TTY0000028", "").Error()
	require.Contains(t, msg, workorder.CausePhraseStdWeight)
	assert.NotContains(t, msg, "on machine")
}
