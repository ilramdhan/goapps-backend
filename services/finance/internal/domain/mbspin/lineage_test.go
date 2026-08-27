// Package mbspin_test covers the pure duplicate/lineage rules (phase P8).
//
// Everything here runs with NO database: AssertNoParentCycle takes its single-hop
// read as an injected ParentLookup, so the whole rule is exercised against
// in-memory chains. The postgres-backed behavior belongs in an integration test.
package mbspin_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// chainLookup builds a ParentLookup from an explicit child -> parent map. A key
// that is absent means "no parent", matching the nil-terminates contract.
func chainLookup(m map[uuid.UUID]uuid.UUID) mbspin.ParentLookup {
	return func(id uuid.UUID) (*uuid.UUID, error) {
		parent, ok := m[id]
		if !ok {
			return nil, nil
		}
		p := parent
		return &p, nil
	}
}

func TestAssertNoParentCycle_NoParent_DepthZero(t *testing.T) {
	src := uuid.New()
	depth, err := mbspin.AssertNoParentCycle(src, chainLookup(nil))
	require.NoError(t, err)
	assert.Equal(t, 0, depth)
}

func TestAssertNoParentCycle_ShortChain_ReturnsDepth(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	// a -> b -> c -> (none)
	depth, err := mbspin.AssertNoParentCycle(a, chainLookup(map[uuid.UUID]uuid.UUID{a: b, b: c}))
	require.NoError(t, err)
	assert.Equal(t, 2, depth)
}

// A -> A. The DB permits this (migration 000484 ships no chk_mbs_parent_not_self),
// so the domain rule is the only thing that rejects it.
func TestAssertNoParentCycle_SelfLoop(t *testing.T) {
	a := uuid.New()
	_, err := mbspin.AssertNoParentCycle(a, chainLookup(map[uuid.UUID]uuid.UUID{a: a}))
	assert.ErrorIs(t, err, mbspin.ErrParentCycle)
}

// A -> B -> A: the multi-hop cycle back to the source being duplicated.
func TestAssertNoParentCycle_TwoHopCycleBackToSource(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	_, err := mbspin.AssertNoParentCycle(a, chainLookup(map[uuid.UUID]uuid.UUID{a: b, b: a}))
	assert.ErrorIs(t, err, mbspin.ErrParentCycle)
}

// A -> B -> C -> B: a loop upstream that does NOT include the source. Without the
// seen-set this would spin until the depth budget ran out and be misreported as
// ErrMaxDuplicateDepth; it must be reported as the cycle it is.
func TestAssertNoParentCycle_UpstreamLoopNotIncludingSource(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	_, err := mbspin.AssertNoParentCycle(a, chainLookup(map[uuid.UUID]uuid.UUID{a: b, b: c, c: b}))
	assert.ErrorIs(t, err, mbspin.ErrParentCycle)
}

// A chain of exactly MaxLineageDepth hops is legal; one more is not.
func TestAssertNoParentCycle_DepthBoundary(t *testing.T) {
	build := func(hops int) (uuid.UUID, mbspin.ParentLookup) {
		ids := make([]uuid.UUID, hops+1)
		for i := range ids {
			ids[i] = uuid.New()
		}
		m := make(map[uuid.UUID]uuid.UUID, hops)
		for i := 0; i < hops; i++ {
			m[ids[i]] = ids[i+1]
		}
		return ids[0], chainLookup(m)
	}

	src, lookup := build(mbspin.MaxLineageDepth)
	depth, err := mbspin.AssertNoParentCycle(src, lookup)
	require.NoError(t, err, "a chain of exactly MaxLineageDepth hops must be accepted")
	assert.Equal(t, mbspin.MaxLineageDepth, depth)

	src, lookup = build(mbspin.MaxLineageDepth + 1)
	_, err = mbspin.AssertNoParentCycle(src, lookup)
	assert.ErrorIs(t, err, mbspin.ErrMaxDuplicateDepth)
}

func TestAssertNoParentCycle_NilSourceRejected(t *testing.T) {
	_, err := mbspin.AssertNoParentCycle(uuid.Nil, chainLookup(nil))
	assert.ErrorIs(t, err, mbspin.ErrInvalidHeadID)
}

// A lookup failure must surface unchanged, not be swallowed into "no parent"
// (which would let a duplicate proceed past an unverified chain).
func TestAssertNoParentCycle_LookupErrorPropagates(t *testing.T) {
	boom := errors.New("connection reset")
	_, err := mbspin.AssertNoParentCycle(uuid.New(), func(uuid.UUID) (*uuid.UUID, error) { return nil, boom })
	assert.ErrorIs(t, err, boom)
}

func TestCloneMgtName_SuffixesWhenNoOverride(t *testing.T) {
	got, err := mbspin.CloneMgtName("SPIN-A", nil)
	require.NoError(t, err)
	assert.Equal(t, "SPIN-A (copy)", got)
}

func TestCloneMgtName_OverrideWinsVerbatim(t *testing.T) {
	override := "SPIN-B"
	got, err := mbspin.CloneMgtName("SPIN-A", &override)
	require.NoError(t, err)
	assert.Equal(t, "SPIN-B", got)
}

func TestCloneMgtName_OverrideValidatedLikeUpdate(t *testing.T) {
	empty := ""
	_, err := mbspin.CloneMgtName("SPIN-A", &empty)
	assert.ErrorIs(t, err, mbspin.ErrEmptyMgtName)

	tooLong := string(make([]byte, 101))
	_, err = mbspin.CloneMgtName("SPIN-A", &tooLong)
	assert.ErrorIs(t, err, mbspin.ErrMgtNameTooLong)
}

// The suffix must never push the name past VARCHAR(100); the source name is kept
// whole rather than truncated.
func TestCloneMgtName_SuffixSkippedRatherThanTruncated(t *testing.T) {
	long := string(make([]byte, 98))
	got, err := mbspin.CloneMgtName(long, nil)
	require.NoError(t, err)
	assert.Equal(t, long, got)
	assert.LessOrEqual(t, len(got), 100)

	fits := string(make([]byte, 93)) // 93 + len(" (copy)") == 100
	got, err = mbspin.CloneMgtName(fits, nil)
	require.NoError(t, err)
	assert.Len(t, got, 100)
}

func TestAssertRecalcFanOut(t *testing.T) {
	require.NoError(t, mbspin.AssertRecalcFanOut(0))
	require.NoError(t, mbspin.AssertRecalcFanOut(mbspin.MaxRecalcChildren))
	err := mbspin.AssertRecalcFanOut(mbspin.MaxRecalcChildren + 1)
	assert.ErrorIs(t, err, mbspin.ErrTooManyChildren)
}

// =============================================================================
// Lineage hydration
// =============================================================================

func newBareSpin(t *testing.T) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), "SPIN-L", nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	return e
}

// A legacy Oracle row carries NULL in all six columns; nothing may be defaulted
// into a non-nil value, and IsClone must stay false.
func TestHydrateLineage_LegacyRowStaysAllNil(t *testing.T) {
	e := newBareSpin(t)
	e.HydrateLineage(mbspin.Lineage{})

	assert.Nil(t, e.ParentSpinID())
	assert.Nil(t, e.DuplicatedAt())
	assert.Nil(t, e.DuplicatedBy())
	assert.Nil(t, e.LastRecalcAt())
	assert.Nil(t, e.LastRecalcBy())
	assert.Nil(t, e.CostProductID())
	assert.False(t, e.IsClone())
}

func TestHydrateLineage_RoundTripsAllSixColumns(t *testing.T) {
	parent := uuid.New()
	dupAt := time.Now().Add(-2 * time.Hour)
	recAt := time.Now().Add(-time.Hour)
	actor := "duplicator"
	recalcActor := "recalcer"
	var costProduct int64 = 4242

	e := newBareSpin(t)
	e.HydrateLineage(mbspin.Lineage{
		ParentSpinID:  &parent,
		DuplicatedAt:  &dupAt,
		DuplicatedBy:  &actor,
		LastRecalcAt:  &recAt,
		LastRecalcBy:  &recalcActor,
		CostProductID: &costProduct,
	})

	require.NotNil(t, e.ParentSpinID())
	assert.Equal(t, parent, *e.ParentSpinID())
	assert.Equal(t, dupAt, *e.DuplicatedAt())
	assert.Equal(t, actor, *e.DuplicatedBy())
	assert.Equal(t, recAt, *e.LastRecalcAt())
	assert.Equal(t, recalcActor, *e.LastRecalcBy())
	assert.Equal(t, costProduct, *e.CostProductID())
	assert.True(t, e.IsClone())
}

// IsRnD is the A6/A7 candidate predicate: only the exact "R and D" spelling is a
// recalc candidate.
func TestIsRnD_OnlyExactStatusIsCandidate(t *testing.T) {
	cases := []struct {
		name   string
		status *string
		want   bool
	}{
		{"nil status is not a candidate", nil, false},
		{"exact R and D", ptr(mbspin.StatusRnD), true},
		{"Spinning", ptr("Spinning"), false},
		{"Boughtout", ptr("Boughtout"), false},
		{"RND without spaces", ptr("RND"), false},
		{"R&D ampersand", ptr("R&D"), false},
		{"lowercase", ptr("r and d"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := mbspin.New(uuid.New(), "SPIN-S", nil, nil, nil, nil, nil, nil, nil, nil,
				tc.status, nil, nil, nil, nil, nil, "admin")
			require.NoError(t, err)
			assert.Equal(t, tc.want, e.IsRnD())
		})
	}
}

// StatusRnD must stay byte-identical to the persisted Oracle value; a silent
// respelling would make every clone a non-candidate for recalc.
func TestStatusRnD_ExactSpelling(t *testing.T) {
	assert.Equal(t, "R and D", mbspin.StatusRnD)
}

// A clone is born explicitly NOT fixed. NULL would read as fixed and would
// permanently exclude the clone from recalc (§11 item 95) — this asserts the
// domain-side interpretation the duplicate INSERT relies on.
func TestCloneFixedMarkers_ExplicitFalseIsNotFixed(t *testing.T) {
	no := false
	e, err := mbspin.New(uuid.New(), "SPIN-C", nil, nil, nil, nil, nil, nil, nil, nil,
		ptr(mbspin.StatusRnD), nil, nil, nil, &no, &no, "admin")
	require.NoError(t, err)

	assert.False(t, e.IsFixedLDR())
	assert.False(t, e.IsFixedDozing())

	nulled := newBareSpin(t)
	assert.True(t, nulled.IsFixedLDR(), "NULL must read as fixed — this is why the clone writes FALSE")
	assert.True(t, nulled.IsFixedDozing())
}

func ptr[T any](v T) *T { return &v }
