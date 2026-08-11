// Package spinfixedcost_test provides unit tests for the anchor-row guard.
//
// The guard is the only thing standing between a routine master-data delete and
// ~4,003 POY products silently costing zero fixed cost, so it is tested exhaustively.
package spinfixedcost_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

// candidateRow builds a row in the state the guard inspects.
func candidateRow(period string, isActive, isDeleted bool) *spinfixedcost.Entity {
	in := spinfixedcost.ReconstructInput{
		ID:              uuid.New(),
		Period:          period,
		CommonPoyDenier: 150,
		PoyProduction:   1_200_000,
		IsActive:        isActive,
		CreatedAt:       time.Now(),
		CreatedBy:       testActor,
	}
	if isDeleted {
		now := time.Now()
		by := "remover"
		in.DeletedAt = &now
		in.DeletedBy = &by
	}
	return spinfixedcost.Reconstruct(in)
}

func TestCheckAnchorGuard(t *testing.T) {
	tests := []struct {
		name      string
		candidate *spinfixedcost.Entity
		stats     spinfixedcost.AnchorStats
		wantErr   error
	}{
		{
			// An inactive row is already invisible to the calc engine's
			// msfc_is_active = TRUE predicate, so removing it changes nothing.
			name:      "candidate already inactive - allowed even as sole row",
			candidate: candidateRow("202606", false, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          0,
				EarliestRemainingActivePeriod: "",
				HasLiveRowAfterCandidate:      false,
			},
			wantErr: nil,
		},
		{
			name:      "candidate already inactive - allowed even as earliest with later rows",
			candidate: candidateRow("202601", false, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          2,
				EarliestRemainingActivePeriod: "202606",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: nil,
		},
		{
			name:      "candidate already deleted - allowed",
			candidate: candidateRow("202601", true, true),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          0,
				EarliestRemainingActivePeriod: "",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: nil,
		},
		{
			name:      "only live+active row - refused",
			candidate: candidateRow("202606", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          0,
				EarliestRemainingActivePeriod: "",
				HasLiveRowAfterCandidate:      false,
			},
			wantErr: spinfixedcost.ErrAnchorRowOnly,
		},
		{
			// RemainingActiveCount == 0 wins even if some inactive live row sits later.
			name:      "only live+active row with a later inactive row - still ErrAnchorRowOnly",
			candidate: candidateRow("202601", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          0,
				EarliestRemainingActivePeriod: "",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: spinfixedcost.ErrAnchorRowOnly,
		},
		{
			name:      "earliest live+active row while later live rows exist - refused",
			candidate: candidateRow("202601", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          3,
				EarliestRemainingActivePeriod: "202604",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: spinfixedcost.ErrAnchorRowEarliest,
		},
		{
			name:      "earliest across a year rollover - refused",
			candidate: candidateRow("202512", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          2,
				EarliestRemainingActivePeriod: "202601",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: spinfixedcost.ErrAnchorRowEarliest,
		},
		{
			name:      "middle row with rows before and after - allowed",
			candidate: candidateRow("202606", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          4,
				EarliestRemainingActivePeriod: "202601",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: nil,
		},
		{
			name:      "latest row while earlier rows exist - allowed",
			candidate: candidateRow("202612", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          5,
				EarliestRemainingActivePeriod: "202601",
				HasLiveRowAfterCandidate:      false,
			},
			wantErr: nil,
		},
		{
			name:      "two rows - removing the earlier is refused",
			candidate: candidateRow("202609", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          1,
				EarliestRemainingActivePeriod: "202610",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: spinfixedcost.ErrAnchorRowEarliest,
		},
		{
			name:      "two rows - removing the later is allowed",
			candidate: candidateRow("202610", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          1,
				EarliestRemainingActivePeriod: "202609",
				HasLiveRowAfterCandidate:      false,
			},
			wantErr: nil,
		},
		{
			// Defensive: an earliest candidate with nothing live after it leaves the
			// remaining rows fully anchored by themselves.
			name:      "earliest but no live row after candidate - allowed",
			candidate: candidateRow("202601", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          2,
				EarliestRemainingActivePeriod: "202604",
				HasLiveRowAfterCandidate:      false,
			},
			wantErr: nil,
		},
		{
			// Inconsistent stats (count > 0 but no earliest period) must fail closed,
			// not open: the empty-string branch treats the candidate as the earliest.
			name:      "positive count with empty earliest period and later rows - refused",
			candidate: candidateRow("202606", true, false),
			stats: spinfixedcost.AnchorStats{
				RemainingActiveCount:          1,
				EarliestRemainingActivePeriod: "",
				HasLiveRowAfterCandidate:      true,
			},
			wantErr: spinfixedcost.ErrAnchorRowEarliest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := spinfixedcost.CheckAnchorGuard(tt.candidate, tt.stats)

			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAnchorGuardErrors_ContainInvalidSubstring pins a coupling that is easy to break
// silently: internal/delivery/grpc/error_response.go string-matches "invalid" to map a
// domain error onto HTTP 400. Without the substring these guard refusals surface as a
// 500 Internal Server Error, which reads like a backend bug rather than a rejected edit.
func TestAnchorGuardErrors_ContainInvalidSubstring(t *testing.T) {
	guardErrors := []error{
		spinfixedcost.ErrAnchorRowOnly,
		spinfixedcost.ErrAnchorRowEarliest,
	}

	for _, err := range guardErrors {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid",
			"guard error must contain \"invalid\" so the gRPC mapper returns 400, not 500")
	}
}

// TestDomainErrors_MapToExpectedStatusSubstrings covers the rest of the sentinel set
// against the same string-matching mapper.
func TestDomainErrors_MapToExpectedStatusSubstrings(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		substring string
	}{
		{"not found -> 404", spinfixedcost.ErrNotFound, "not found"},
		{"duplicate period -> 409", spinfixedcost.ErrDuplicatePeriod, "already exists"},
		{"invalid period -> 400", spinfixedcost.ErrInvalidPeriod, "invalid"},
		{"negative amount -> 400", spinfixedcost.ErrNegativeAmount, "invalid"},
		{"non-positive denier -> 400", spinfixedcost.ErrNonPositiveDenier, "invalid"},
		{"non-positive production -> 400", spinfixedcost.ErrNonPositiveProduction, "invalid"},
		{"empty created_by -> 400", spinfixedcost.ErrEmptyCreatedBy, "invalid"},
		{"already deleted -> 400", spinfixedcost.ErrAlreadyDeleted, "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, tt.err)
			assert.Contains(t, strings.ToLower(tt.err.Error()), tt.substring)
		})
	}
}

// TestAnchorGuardErrors_AreDistinctSentinels makes sure the two refusals cannot be
// confused by errors.Is, so callers can tell "last row" from "earliest row".
func TestAnchorGuardErrors_AreDistinctSentinels(t *testing.T) {
	assert.False(t, errors.Is(spinfixedcost.ErrAnchorRowOnly, spinfixedcost.ErrAnchorRowEarliest))
	assert.False(t, errors.Is(spinfixedcost.ErrAnchorRowEarliest, spinfixedcost.ErrAnchorRowOnly))
}
