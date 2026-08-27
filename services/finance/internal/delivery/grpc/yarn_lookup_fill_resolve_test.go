package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// resolveFillFakeRepo is a call-tracking mbspin.Repository test double dedicated
// to proving resolveMBSpinForFill's dispatch order: permanent-ID lookup first
// (when selectedKey parses as a UUID), falling through UNCHANGED to the legacy
// ORION-code / mb_costing chain otherwise. Every method not exercised by these
// tests is an inert stub, present only to satisfy the interface.
type resolveFillFakeRepo struct {
	byID map[string]*mbspin.Entity // keyed by uuid.String()

	byOrionResult *mbspin.Entity
	byOrionErr    error
	byOrionCalls  []string

	byMBCostingResult *mbspin.Entity
	byMBCostingErr    error
	byMBCostingCalls  []string
}

func (r *resolveFillFakeRepo) Create(_ context.Context, _ *mbspin.Entity) error { return nil }

func (r *resolveFillFakeRepo) GetByID(_ context.Context, id uuid.UUID) (*mbspin.Entity, error) {
	if e, ok := r.byID[id.String()]; ok {
		return e, nil
	}
	return nil, mbspin.ErrNotFound
}

func (r *resolveFillFakeRepo) List(_ context.Context, _ mbspin.ListFilter) ([]*mbspin.Entity, int64, error) {
	return nil, 0, nil
}
func (r *resolveFillFakeRepo) Update(_ context.Context, _ *mbspin.Entity) error { return nil }
func (r *resolveFillFakeRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *resolveFillFakeRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (r *resolveFillFakeRepo) GetByMBCosting(_ context.Context, code string) (*mbspin.Entity, error) {
	r.byMBCostingCalls = append(r.byMBCostingCalls, code)
	return r.byMBCostingResult, r.byMBCostingErr
}

func (r *resolveFillFakeRepo) GetByOrionItemCode(_ context.Context, code string) (*mbspin.Entity, error) {
	r.byOrionCalls = append(r.byOrionCalls, code)
	return r.byOrionResult, r.byOrionErr
}

func (r *resolveFillFakeRepo) DuplicateSpin(_ context.Context, _ mbspin.DuplicateInput) (mbspin.DuplicateOutput, error) {
	return mbspin.DuplicateOutput{}, nil
}
func (r *resolveFillFakeRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]*mbspin.Entity, error) {
	return nil, nil
}
func (r *resolveFillFakeRepo) ExistsByOrionItemCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *resolveFillFakeRepo) ResolveUniqueByOrionItemCode(_ context.Context, _ string) (uuid.UUID, bool, error) {
	return uuid.UUID{}, false, nil
}

func newResolveTestSpin(t *testing.T, name string) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), name, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	return e
}

// TestResolveMBSpinForFill_ValidUUID_ResolvesByPermanentID_WithoutTouchingLegacyChain
// proves Task 2's new branch: a selectedKey that IS the permanent mst_mb_spin.mbs_id
// (e.g. the value carried by cpp_value_mb_spin_id) resolves via GetByID and never
// even calls the legacy ORION-code / mb_costing lookups.
func TestResolveMBSpinForFill_ValidUUID_ResolvesByPermanentID_WithoutTouchingLegacyChain(t *testing.T) {
	spin := newResolveTestSpin(t, "SPIN-BY-ID")
	repo := &resolveFillFakeRepo{byID: map[string]*mbspin.Entity{spin.ID().String(): spin}}
	h := &YarnLookupFillHandler{mbSpinRepo: repo}

	got, err := h.resolveMBSpinForFill(context.Background(), spin.ID().String())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, spin.ID(), got.ID())
	assert.Empty(t, repo.byOrionCalls, "permanent-ID hit must not fall through to the ORION lookup")
	assert.Empty(t, repo.byMBCostingCalls, "permanent-ID hit must not fall through to the mb_costing lookup")
}

// TestResolveMBSpinForFill_UUIDShapedButUnknown_FallsThroughToLegacyChain proves that
// a UUID-shaped selectedKey with no matching spin does NOT error out early — it falls
// through to the same legacy chain a non-UUID key would use.
func TestResolveMBSpinForFill_UUIDShapedButUnknown_FallsThroughToLegacyChain(t *testing.T) {
	unknown := uuid.New()
	legacySpin := newResolveTestSpin(t, "SPIN-LEGACY-FALLBACK")
	repo := &resolveFillFakeRepo{
		byID:          map[string]*mbspin.Entity{}, // GetByID will report ErrNotFound
		byOrionResult: legacySpin,
	}
	h := &YarnLookupFillHandler{mbSpinRepo: repo}

	got, err := h.resolveMBSpinForFill(context.Background(), unknown.String())

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, legacySpin.ID(), got.ID())
	require.Len(t, repo.byOrionCalls, 1)
	assert.Equal(t, unknown.String(), repo.byOrionCalls[0])
}

// TestResolveMBSpinForFill_LegacyKey_BehavesExactlyAsBefore is the no-regression guard
// for legacy rows: a non-UUID selectedKey (ORION code or mb_costing code) must reach
// the ORION lookup first, then mb_costing on failure — completely unchanged from
// before this task's GetByID branch existed.
func TestResolveMBSpinForFill_LegacyKey_BehavesExactlyAsBefore(t *testing.T) {
	t.Run("ORION code hit — mb_costing never called", func(t *testing.T) {
		legacySpin := newResolveTestSpin(t, "SPIN-ORION-HIT")
		repo := &resolveFillFakeRepo{byOrionResult: legacySpin}
		h := &YarnLookupFillHandler{mbSpinRepo: repo}

		got, err := h.resolveMBSpinForFill(context.Background(), "ORION-CODE-123")

		require.NoError(t, err)
		assert.Equal(t, legacySpin.ID(), got.ID())
		assert.Empty(t, repo.byMBCostingCalls)
	})

	t.Run("ORION miss falls back to mb_costing", func(t *testing.T) {
		legacySpin := newResolveTestSpin(t, "SPIN-MBCOSTING-HIT")
		repo := &resolveFillFakeRepo{
			byOrionErr:        mbspin.ErrNotFound,
			byMBCostingResult: legacySpin,
		}
		h := &YarnLookupFillHandler{mbSpinRepo: repo}

		got, err := h.resolveMBSpinForFill(context.Background(), "MBCOSTING-CODE-456")

		require.NoError(t, err)
		assert.Equal(t, legacySpin.ID(), got.ID())
		require.Len(t, repo.byMBCostingCalls, 1)
		assert.Equal(t, "MBCOSTING-CODE-456", repo.byMBCostingCalls[0])
	})

	t.Run("both miss returns the mb_costing error unchanged", func(t *testing.T) {
		repo := &resolveFillFakeRepo{
			byOrionErr:     mbspin.ErrNotFound,
			byMBCostingErr: mbspin.ErrNotFound,
		}
		h := &YarnLookupFillHandler{mbSpinRepo: repo}

		_, err := h.resolveMBSpinForFill(context.Background(), "NO-SUCH-KEY")

		require.ErrorIs(t, err, mbspin.ErrNotFound)
	})
}
