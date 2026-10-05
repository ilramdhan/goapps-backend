package erpintegration

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	t0       = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	t1       = t0.Add(time.Hour)
	testHash = strings.Repeat("ab", 32)
)

func testTotals(t *testing.T) ControlTotals {
	t.Helper()
	ct, err := NewControlTotals(3, decimal.RequireFromString("1.234565"),
		decimal.RequireFromString("0.5"), decimal.RequireFromString("-0.000005"))
	require.NoError(t, err)
	return ct
}

// reconBatch rebuilds a batch in the given status; withDerivation sets rule
// hash + totals (and a push stamp for statuses at or after PUSHED).
func reconBatch(t *testing.T, mode BatchMode, st BatchStatus, withDerivation bool) *Batch {
	t.Helper()
	s := BatchState{ID: 7, Period: "202608", Seq: 1, Mode: mode, Status: st,
		CreatedAt: t0, CreatedBy: "c", UpdatedAt: t0, UpdatedBy: "c"}
	if withDerivation {
		ct := testTotals(t)
		s.RuleHash, s.Totals, s.RuleSnapshot = testHash, &ct, []byte(`{"r":1}`)
	}
	switch st {
	case StatusPushed, StatusValuated, StatusReconciled, StatusLocked, StatusSuperseded:
		s.Pushed = &ActorStamp{At: t0, By: "p"}
	}
	if mode == ModeShadow && !st.AllowedInShadow() {
		t.Fatalf("invalid fixture: shadow in %s", st)
	}
	b, err := ReconstituteBatch(s)
	require.NoError(t, err)
	return b
}

func TestNewBatch(t *testing.T) {
	b, err := NewBatch("202608", ModeLive, "  alice ", t0)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, b.Status())
	assert.Equal(t, "202608", b.Period())
	assert.Equal(t, ModeLive, b.Mode())
	assert.Equal(t, int64(0), b.ID())
	assert.Equal(t, 0, b.Seq())
	assert.Equal(t, "alice", b.CreatedBy())
	assert.Equal(t, "alice", b.UpdatedBy())
	assert.Equal(t, t0, b.CreatedAt())
	assert.Equal(t, t0, b.UpdatedAt())
	assert.False(t, b.IsFrozen())
	assert.Nil(t, b.Totals())
	assert.Empty(t, b.RuleHash())
	assert.Nil(t, b.DemandLoadedAt())
	assert.False(t, b.NeedsRepush())
}

func TestNewBatchValidation(t *testing.T) {
	_, err := NewBatch("202613", ModeLive, "a", t0)
	assert.ErrorIs(t, err, ErrInvalidBatchPeriod)
	_, err = NewBatch("202608", "TEST", "a", t0)
	assert.ErrorIs(t, err, ErrInvalidBatchMode)
	_, err = NewBatch("202608", ModeLive, "  ", t0)
	assert.ErrorIs(t, err, ErrActorRequired)
	_, err = NewBatch("202608", ModeLive, strings.Repeat("x", 65), t0)
	assert.ErrorIs(t, err, ErrActorRequired)
}

func TestReconstituteValidation(t *testing.T) {
	base := BatchState{ID: 1, Period: "202608", Seq: 1, Mode: ModeLive, Status: StatusDraft}
	cases := []struct {
		name string
		mut  func(*BatchState)
		want error
	}{
		{"period", func(s *BatchState) { s.Period = "2026" }, ErrInvalidBatchPeriod},
		{"seq", func(s *BatchState) { s.Seq = 0 }, ErrInvalidBatchSeq},
		{"mode", func(s *BatchState) { s.Mode = "x" }, ErrInvalidBatchMode},
		{"status", func(s *BatchState) { s.Status = "x" }, ErrInvalidBatchStatus},
		{"shadow pushed", func(s *BatchState) { s.Mode = ModeShadow; s.Status = StatusPushed }, ErrShadowNotPushable},
		{"hash", func(s *BatchState) { s.RuleHash = "ABC" }, ErrInvalidRuleHash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.mut(&s)
			b, err := ReconstituteBatch(s)
			assert.Nil(t, b)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	ct := testTotals(t)
	dl := t0
	rc := t1
	s := BatchState{
		ID: 9, Period: "202607", Seq: 2, Mode: ModeLive, Status: StatusReconciled,
		JobID: "job-1", DemandLoadedAt: &dl, RuleSnapshot: []byte(`{}`), RuleHash: testHash,
		Totals: &ct, Summary: []byte(`{"s":1}`),
		WarningsAck: &ActorStamp{At: t0, By: "w"}, Pushed: &ActorStamp{At: t0, By: "p"},
		Valuated: &ActorStamp{At: t0, By: "v"}, ReconciledAt: &rc,
		AdjApproved: &ActorStamp{At: t1, By: "ap"}, Locked: nil,
		NeedsRepush: false, Error: "", CreatedAt: t0, CreatedBy: "c", UpdatedAt: t1, UpdatedBy: "u",
	}
	b, err := ReconstituteBatch(s)
	require.NoError(t, err)
	assert.Equal(t, s, b.State())
	assert.Equal(t, "job-1", b.JobID())
	assert.Equal(t, []byte(`{}`), b.RuleSnapshot())
	assert.Equal(t, []byte(`{"s":1}`), b.Summary())
	assert.Equal(t, "w", b.WarningsAck().By)
	assert.Equal(t, "p", b.Pushed().By)
	assert.Equal(t, "v", b.Valuated().By)
	assert.Equal(t, rc, *b.ReconciledAt())
	assert.Equal(t, "ap", b.AdjApproved().By)
	assert.Nil(t, b.Locked())
	assert.Equal(t, dl, *b.DemandLoadedAt())
	assert.True(t, b.IsFrozen())

	// Getters return copies: mutating them must not affect the aggregate.
	b.RuleSnapshot()[0] = 'X'
	b.Totals().rowCount = 99
	b.Pushed().By = "hack"
	assert.Equal(t, []byte(`{}`), b.RuleSnapshot())
	assert.Equal(t, int64(3), b.Totals().RowCount())
	assert.Equal(t, "p", b.Pushed().By)
}

func TestControlTotals(t *testing.T) {
	ct := testTotals(t)
	assert.Equal(t, int64(3), ct.RowCount())
	assert.Equal(t, "1.23457", ct.SumStd().StringFixed(5), "half away from zero")
	assert.True(t, ct.SumConv().Equal(decimal.RequireFromString("0.5")))
	assert.Equal(t, "-0.00001", ct.SumPvl().StringFixed(5))
	assert.True(t, ct.Equal(testTotals(t)))
	other, err := NewControlTotals(3, decimal.RequireFromString("1.23457"),
		decimal.RequireFromString("0.50000"), decimal.RequireFromString("-0.00001"))
	require.NoError(t, err)
	assert.True(t, ct.Equal(other), "decimal-aware equality")
	diff, err := NewControlTotals(4, ct.SumStd(), ct.SumConv(), ct.SumPvl())
	require.NoError(t, err)
	assert.False(t, ct.Equal(diff))
	_, err = NewControlTotals(-1, decimal.Zero, decimal.Zero, decimal.Zero)
	assert.ErrorIs(t, err, ErrInvalidControlTotals)
}

// TestHappyPath walks DRAFT -> LOCKED and checks every stamp.
func TestHappyPath(t *testing.T) {
	b, err := NewBatch("202608", ModeLive, "alice", t0)
	require.NoError(t, err)

	inv, err := b.Transition(StatusDemandLoaded, "alice", t1)
	require.NoError(t, err)
	assert.True(t, inv.Demand)
	require.NotNil(t, b.DemandLoadedAt())
	assert.Equal(t, t1, *b.DemandLoadedAt())

	_, err = b.Transition(StatusCovered, "alice", t1)
	require.NoError(t, err)
	_, err = b.Transition(StatusDerived, "alice", t1)
	require.NoError(t, err)
	require.NoError(t, b.SetDerivation([]byte(`{"a":1}`), testHash, testTotals(t), "alice", t1))
	require.NoError(t, b.AckWarnings("bob", t1))
	_, err = b.Transition(StatusValidated, "alice", t1)
	require.NoError(t, err)
	require.NotNil(t, b.WarningsAck(), "validate keeps the ack")
	// Re-verification at validate is allowed.
	require.NoError(t, b.SetDerivation([]byte(`{"a":1}`), testHash, testTotals(t), "alice", t1))

	_, err = b.Transition(StatusPushed, "pusher", t1)
	require.NoError(t, err)
	assert.True(t, b.IsFrozen())
	assert.Equal(t, "pusher", b.Pushed().By)
	assert.ErrorIs(t, b.SetDerivation(nil, testHash, testTotals(t), "x", t1), ErrBatchFrozen)

	_, err = b.Transition(StatusPushed, "pusher", t1) // valuation refused, stays
	require.NoError(t, err)
	_, err = b.Transition(StatusValuated, "val", t1)
	require.NoError(t, err)
	assert.Equal(t, "val", b.Valuated().By)
	_, err = b.Transition(StatusValuated, "val", t1) // recon with DIFF
	require.NoError(t, err)
	_, err = b.Transition(StatusReconciled, "rec", t1)
	require.NoError(t, err)
	require.NotNil(t, b.ReconciledAt())

	require.NoError(t, b.MarkAdjApproved("appr", t1))
	assert.Equal(t, StatusReconciled, b.Status(), "approval does not change status (D-D2)")
	assert.Equal(t, "appr", b.AdjApproved().By)

	_, err = b.Transition(StatusLocked, "locker", t1)
	require.NoError(t, err)
	assert.Equal(t, "locker", b.Locked().By)
	assert.Equal(t, testHash, b.RuleHash(), "hash preserved to LOCKED")
	assert.Empty(t, b.AllowedTransitions())
	assert.Equal(t, "locker", b.UpdatedBy())
}

func TestRerunInvalidatesDownstream(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusValidated, true)
	require.NoError(t, b.AckWarnings("w", t0))

	// Re-derive from VALIDATED clears the ack and the derivation.
	inv, err := b.Transition(StatusDerived, "u", t1)
	require.NoError(t, err)
	assert.True(t, inv.StdRows && inv.WarningsAck && inv.RuleSnapshot)
	assert.Nil(t, b.WarningsAck())
	assert.Empty(t, b.RuleHash())
	assert.Nil(t, b.Totals())
	assert.Nil(t, b.RuleSnapshot())

	// Reload clears everything and re-stamps demand_loaded_at.
	b = reconBatch(t, ModeLive, StatusValidated, true)
	inv, err = b.Transition(StatusDemandLoaded, "u", t1)
	require.NoError(t, err)
	assert.True(t, inv.Demand && inv.Coverage)
	assert.Empty(t, b.RuleHash())
	assert.Equal(t, t1, *b.DemandLoadedAt())
}

func TestPushRequiresDerivation(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusValidated, false)
	_, err := b.Transition(StatusPushed, "u", t1)
	assert.ErrorIs(t, err, ErrControlTotalsMissing)
	assert.Equal(t, StatusValidated, b.Status())
	assert.NotContains(t, b.AllowedTransitions(), StatusPushed)
}

func TestReReconClearsReconciledAt(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusValuated, true)
	_, err := b.Transition(StatusReconciled, "u", t1)
	require.NoError(t, err)
	_, err = b.Transition(StatusValuated, "u", t1)
	require.NoError(t, err)
	assert.Nil(t, b.ReconciledAt())
}

func TestShadowNeverPushes(t *testing.T) {
	b, err := NewBatch("202604", ModeShadow, "bt", t0)
	require.NoError(t, err)
	for _, to := range []BatchStatus{StatusDemandLoaded, StatusCovered, StatusDerived} {
		_, err = b.Transition(to, "bt", t1)
		require.NoError(t, err)
	}
	require.NoError(t, b.SetDerivation([]byte(`{}`), testHash, testTotals(t), "bt", t1))
	_, err = b.Transition(StatusValidated, "bt", t1)
	require.NoError(t, err)
	assert.Equal(t, []BatchStatus{StatusDemandLoaded, StatusDerived, StatusFailed}, b.AllowedTransitions())
	_, err = b.Transition(StatusPushed, "bt", t1)
	assert.ErrorIs(t, err, ErrShadowNotPushable)
	assert.Equal(t, StatusValidated, b.Status())
	require.NoError(t, b.Fail("done", "bt", t1))
	assert.Equal(t, StatusFailed, b.Status())
}

func TestNeedsRepush(t *testing.T) {
	// Not applicable before push.
	b := reconBatch(t, ModeLive, StatusValidated, true)
	changed, err := b.MarkNeedsRepush("u", t1)
	require.NoError(t, err)
	assert.False(t, changed)

	for _, st := range []BatchStatus{StatusPushed, StatusValuated, StatusReconciled} {
		t.Run(string(st), func(t *testing.T) {
			b := reconBatch(t, ModeLive, st, true)
			changed, err := b.MarkNeedsRepush("u", t1)
			require.NoError(t, err)
			assert.True(t, changed)
			assert.True(t, b.NeedsRepush())
			changed, err = b.MarkNeedsRepush("u", t1)
			require.NoError(t, err)
			assert.False(t, changed, "idempotent")
			for _, to := range []BatchStatus{StatusValuated, StatusReconciled, StatusLocked} {
				if CanTransition(st, to) {
					_, err := b.Transition(to, "u", t1)
					assert.ErrorIs(t, err, ErrNeedsRepush, "%s->%s", st, to)
				}
				assert.NotContains(t, b.AllowedTransitions(), to)
			}
			assert.Contains(t, b.AllowedTransitions(), StatusFailed)
			require.NoError(t, b.Fail("repush", "u", t1))
		})
	}

	b = reconBatch(t, ModeLive, StatusReconciled, true)
	_, err = b.MarkNeedsRepush("u", t1)
	require.NoError(t, err)
	assert.ErrorIs(t, b.MarkAdjApproved("u", t1), ErrNeedsRepush)

	_, err = b.MarkNeedsRepush(" ", t1)
	assert.ErrorIs(t, err, ErrActorRequired)
}

func TestFail(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusPushed, true)
	require.NoError(t, b.Fail("  abandoned by user ", "u", t1))
	assert.Equal(t, StatusFailed, b.Status())
	assert.Equal(t, "abandoned by user", b.ErrorText())
	// FAILED is terminal: a re-run needs a new batch.
	assert.ErrorIs(t, b.Fail("again", "u", t1), ErrInvalidTransition)
	_, err := b.Transition(StatusDemandLoaded, "u", t1)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestTransitionArgumentErrors(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusDraft, false)
	_, err := b.Transition(StatusDemandLoaded, "", t1)
	assert.ErrorIs(t, err, ErrActorRequired)
	_, err = b.Transition("NOPE", "u", t1)
	assert.ErrorIs(t, err, ErrInvalidBatchStatus)
	assert.Equal(t, StatusDraft, b.Status())
}

func TestSetDerivationGuards(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusCovered, false)
	assert.ErrorIs(t, b.SetDerivation(nil, testHash, testTotals(t), "u", t1), ErrInvalidTransition)
	b = reconBatch(t, ModeLive, StatusDerived, false)
	assert.ErrorIs(t, b.SetDerivation(nil, "short", testTotals(t), "u", t1), ErrInvalidRuleHash)
	assert.ErrorIs(t, b.SetDerivation(nil, strings.ToUpper(testHash), testTotals(t), "u", t1), ErrInvalidRuleHash)
	assert.ErrorIs(t, b.SetDerivation(nil, testHash, testTotals(t), "", t1), ErrActorRequired)
	snap := []byte(`{"k":1}`)
	require.NoError(t, b.SetDerivation(snap, testHash, testTotals(t), "u", t1))
	snap[0] = 'X'
	assert.Equal(t, []byte(`{"k":1}`), b.RuleSnapshot(), "snapshot copied on set")
}

func TestAckAndApproveGuards(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusCovered, false)
	assert.ErrorIs(t, b.AckWarnings("u", t1), ErrInvalidTransition)
	assert.ErrorIs(t, b.AckWarnings("", t1), ErrActorRequired)
	assert.ErrorIs(t, b.MarkAdjApproved("u", t1), ErrInvalidTransition)
	b = reconBatch(t, ModeLive, StatusReconciled, true)
	assert.ErrorIs(t, b.MarkAdjApproved("", t1), ErrActorRequired)
}

func TestSettersAndAllowed(t *testing.T) {
	b := reconBatch(t, ModeLive, StatusDraft, false)
	b.SetJobID("j2")
	b.SetSummary([]byte(`{"x":1}`))
	assert.Equal(t, "j2", b.JobID())
	assert.Equal(t, []byte(`{"x":1}`), b.Summary())
	assert.Equal(t, []BatchStatus{StatusDemandLoaded, StatusFailed}, b.AllowedTransitions())

	b = reconBatch(t, ModeLive, StatusReconciled, true)
	assert.Equal(t, []BatchStatus{StatusValuated, StatusLocked, StatusFailed, StatusSuperseded}, b.AllowedTransitions())
}

func TestValidateBatchPeriod(t *testing.T) {
	for _, p := range []string{"202601", "202612"} {
		assert.NoError(t, ValidateBatchPeriod(p))
	}
	for _, p := range []string{"", "202600", "202613", "2026-01", "abcdef"} {
		assert.ErrorIs(t, ValidateBatchPeriod(p), ErrInvalidBatchPeriod, p)
	}
}
