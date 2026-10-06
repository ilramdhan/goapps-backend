package erpintegration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// expectedMatrix is the design Part 1 §5.3 allowed-transition table written
// out as an 11x11 grid, independent of the production map. Rows are "from",
// columns are "to", both in lifecycle order:
//
//	DR DL CO DE VA PU VD RE LO FA SU
var expectedMatrix = [11][11]int{
	/* DRAFT         */ {0, 1, 0, 0, 0, 0, 0, 0, 0, 1, 0},
	/* DEMAND_LOADED */ {0, 1, 1, 0, 0, 0, 0, 0, 0, 1, 0},
	/* COVERED       */ {0, 1, 1, 1, 0, 0, 0, 0, 0, 1, 0},
	/* DERIVED       */ {0, 1, 0, 1, 1, 0, 0, 0, 0, 1, 0},
	/* VALIDATED     */ {0, 1, 0, 1, 0, 1, 0, 0, 0, 1, 0},
	/* PUSHED        */ {0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 0},
	/* VALUATED      */ {0, 0, 0, 0, 0, 0, 1, 1, 0, 1, 1},
	/* RECONCILED    */ {0, 0, 0, 0, 0, 0, 1, 0, 1, 1, 1},
	/* LOCKED        */ {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	/* FAILED        */ {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	/* SUPERSEDED    */ {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
}

func TestCanTransition_All121Pairs(t *testing.T) {
	statuses := AllBatchStatuses()
	require.Len(t, statuses, 11)
	pairs, allowed := 0, 0
	for i, from := range statuses {
		for j, to := range statuses {
			pairs++
			want := expectedMatrix[i][j] == 1
			if want {
				allowed++
			}
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				assert.Equal(t, want, CanTransition(from, to))
			})
		}
	}
	assert.Equal(t, 121, pairs)
	assert.Equal(t, 28, allowed)
}

// TestBatchTransition_All121Pairs drives the aggregate itself (not only the
// table) through every pair: a LIVE batch reconstituted in `from`, with
// totals and hash set so the push invariant does not interfere.
func TestBatchTransition_All121Pairs(t *testing.T) {
	statuses := AllBatchStatuses()
	for i, from := range statuses {
		for j, to := range statuses {
			want := expectedMatrix[i][j] == 1
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				b := reconBatch(t, ModeLive, from, true)
				_, err := b.Transition(to, "tester", t1)
				if want {
					require.NoError(t, err)
					assert.Equal(t, to, b.Status())
				} else {
					assert.ErrorIs(t, err, ErrInvalidTransition)
					assert.Equal(t, from, b.Status(), "status unchanged on rejection")
				}
			})
		}
	}
}

func TestCanTransition_UnknownStatus(t *testing.T) {
	assert.False(t, CanTransition("BOGUS", StatusDraft))
	assert.False(t, CanTransition(StatusDraft, "BOGUS"))
}

func TestTerminalStatusesHaveNoExits(t *testing.T) {
	for _, from := range AllBatchStatuses() {
		any := false
		for _, to := range AllBatchStatuses() {
			if CanTransition(from, to) {
				any = true
			}
		}
		assert.Equal(t, !from.IsTerminal(), any, from)
	}
}

func TestInvalidationFor(t *testing.T) {
	full := Invalidation{Demand: true, Coverage: true, StdRows: true, RuleSnapshot: true, WarningsAck: true}
	cov := Invalidation{Coverage: true, StdRows: true, RuleSnapshot: true, WarningsAck: true}
	der := Invalidation{StdRows: true, RuleSnapshot: true, WarningsAck: true}
	cases := []struct {
		from, to BatchStatus
		want     Invalidation
	}{
		{StatusDraft, StatusDemandLoaded, full},
		{StatusDemandLoaded, StatusDemandLoaded, full},
		{StatusCovered, StatusDemandLoaded, full},
		{StatusDerived, StatusDemandLoaded, full},
		{StatusValidated, StatusDemandLoaded, full},
		{StatusDemandLoaded, StatusCovered, cov},
		{StatusCovered, StatusCovered, cov},
		{StatusCovered, StatusDerived, der},
		{StatusDerived, StatusDerived, der},
		{StatusValidated, StatusDerived, der},
		{StatusDerived, StatusValidated, Invalidation{}},
		{StatusValidated, StatusPushed, Invalidation{}},
		{StatusValuated, StatusReconciled, Invalidation{}},
		{StatusDraft, StatusCovered, Invalidation{}}, // disallowed
		{StatusLocked, StatusDemandLoaded, Invalidation{}},
	}
	for _, tc := range cases {
		got := InvalidationFor(tc.from, tc.to)
		assert.Equal(t, tc.want, got, "%s->%s", tc.from, tc.to)
		assert.Equal(t, tc.want != (Invalidation{}), got.Any())
	}
}

func TestStatusPredicates(t *testing.T) {
	active := map[BatchStatus]bool{StatusValuated: true, StatusReconciled: true, StatusLocked: true}
	inflight := map[BatchStatus]bool{StatusDraft: true, StatusDemandLoaded: true, StatusCovered: true,
		StatusDerived: true, StatusValidated: true, StatusPushed: true}
	shadow := map[BatchStatus]bool{StatusDraft: true, StatusDemandLoaded: true, StatusCovered: true,
		StatusDerived: true, StatusValidated: true, StatusFailed: true}
	terminal := map[BatchStatus]bool{StatusLocked: true, StatusFailed: true, StatusSuperseded: true}
	for _, s := range AllBatchStatuses() {
		assert.Equal(t, active[s], s.IsActive(), s)
		assert.Equal(t, inflight[s], s.IsInFlight(), s)
		assert.Equal(t, shadow[s], s.AllowedInShadow(), s)
		assert.Equal(t, terminal[s], s.IsTerminal(), s)
		assert.False(t, s.IsActive() && s.IsInFlight(), "active and in-flight are disjoint: %s", s)
		assert.Equal(t, string(s), s.String())
	}
}

func TestParseBatchStatus(t *testing.T) {
	for _, s := range AllBatchStatuses() {
		got, err := ParseBatchStatus(string(s))
		require.NoError(t, err)
		assert.Equal(t, s, got)
	}
	for _, s := range []string{"", "draft", "PUSH", "UNKNOWN", " DRAFT"} {
		_, err := ParseBatchStatus(s)
		assert.ErrorIs(t, err, ErrInvalidBatchStatus, s)
	}
}

func TestParseBatchMode(t *testing.T) {
	m, err := ParseBatchMode("LIVE")
	require.NoError(t, err)
	assert.Equal(t, ModeLive, m)
	m, err = ParseBatchMode("SHADOW")
	require.NoError(t, err)
	assert.Equal(t, ModeShadow, m)
	assert.Equal(t, "SHADOW", m.String())
	for _, s := range []string{"", "live", "TEST"} {
		_, err := ParseBatchMode(s)
		assert.ErrorIs(t, err, ErrInvalidBatchMode, s)
	}
}

func TestAllBatchStatusesIsCopy(t *testing.T) {
	a := AllBatchStatuses()
	a[0] = "X"
	assert.Equal(t, StatusDraft, AllBatchStatuses()[0])
}
