package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// newSpinWithFlags builds a spin entity carrying the given fix/actual markers.
func newSpinWithFlags(t *testing.T, ldrIsFixed, dozingIsFixed *bool) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), "SPIN-A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ldrIsFixed, dozingIsFixed, "admin")
	require.NoError(t, err)
	return e
}

// TestMBSpinEntityToProto_FixedFlags_AbsentStaysAbsent is the absence-vs-zero guard
// (D13): a NULL marker must arrive at the client as absent, never as false. Mapping
// it to false would flip a recalc-protected row into a recalculable one silently.
func TestMBSpinEntityToProto_FixedFlags_AbsentStaysAbsent(t *testing.T) {
	p := mbSpinEntityToProto(newSpinWithFlags(t, nil, nil))

	assert.Nil(t, p.MbsLdrIsFixed, "NULL ldr marker must stay absent, not become false")
	assert.Nil(t, p.MbsDozingIsFixed, "NULL dozing marker must stay absent, not become false")
}

// TestMBSpinEntityToProto_FixedFlags_ExplicitValues proves true and false both
// survive the mapping distinctly — false must not be swallowed as "absent" either.
func TestMBSpinEntityToProto_FixedFlags_ExplicitValues(t *testing.T) {
	yes, no := true, false

	pFixed := mbSpinEntityToProto(newSpinWithFlags(t, &yes, &no))
	require.NotNil(t, pFixed.MbsLdrIsFixed)
	assert.True(t, *pFixed.MbsLdrIsFixed)
	require.NotNil(t, pFixed.MbsDozingIsFixed, "explicit false must be present, not absent")
	assert.False(t, *pFixed.MbsDozingIsFixed)

	pComputed := mbSpinEntityToProto(newSpinWithFlags(t, &no, &yes))
	require.NotNil(t, pComputed.MbsLdrIsFixed)
	assert.False(t, *pComputed.MbsLdrIsFixed)
	require.NotNil(t, pComputed.MbsDozingIsFixed)
	assert.True(t, *pComputed.MbsDozingIsFixed)
}

// TestMBSpinEntityToProto_FixedFlags_MatchDomainPredicate ties the wire values back
// to the domain's recalc-safe interpretation: nil and true both mean FIXED.
func TestMBSpinEntityToProto_FixedFlags_MatchDomainPredicate(t *testing.T) {
	no := false
	e := newSpinWithFlags(t, nil, &no)
	p := mbSpinEntityToProto(e)

	assert.True(t, e.IsFixedLDR())
	assert.Nil(t, p.MbsLdrIsFixed, "absent on the wire, FIXED in the domain")

	assert.False(t, e.IsFixedDozing())
	require.NotNil(t, p.MbsDozingIsFixed)
	assert.False(t, *p.MbsDozingIsFixed)
}

// TestMBSpinEntityToProto_LDRProvenance proves the read-side LDR provenance
// fields (mbs_ldr_type, mbs_ldr_calculated_pct, mbs_ldr_adjustment_pct,
// mbs_ldr_is_actual) round-trip from the domain entity onto the response
// proto, closing the gap where GetMBSpin/ListMBSpins/UpdateMBSpin responses
// previously had no way to expose the current LDR lock/adjustment state.
func TestMBSpinEntityToProto_LDRProvenance(t *testing.T) {
	e := newSpinWithFlags(t, nil, nil)
	calculated := 3.55
	adjustment := 0.25
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{
		LDRType:          mbspin.LDRTypeCalculated,
		LDRCalculatedPct: &calculated,
		LDRAdjustmentPct: &adjustment,
		LDRIsActual:      false,
	})

	p := mbSpinEntityToProto(e)

	assert.Equal(t, mbspin.LDRTypeCalculated, p.MbsLdrType)
	require.NotNil(t, p.MbsLdrCalculatedPct)
	assert.InDelta(t, calculated, *p.MbsLdrCalculatedPct, 0.0001)
	require.NotNil(t, p.MbsLdrAdjustmentPct)
	assert.InDelta(t, adjustment, *p.MbsLdrAdjustmentPct, 0.0001)
	assert.False(t, p.MbsLdrIsActual)
}

// TestMBSpinEntityToProto_ShadeAndCrossSection proves the shade code/name and
// cross section carried down from the parent MB Head (via HydrateShadeAndLDR)
// reach the response proto — this was the gap fixed for P2-T2: the data was
// already persisted on the entity but never mapped onto the wire.
func TestMBSpinEntityToProto_ShadeAndCrossSection(t *testing.T) {
	t.Run("populated values are mapped", func(t *testing.T) {
		e := newSpinWithFlags(t, nil, nil)
		shadeCode := "SC-01"
		shadeName := "Jet Black"
		crossSection := "Round"
		e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{
			ShadeCode:    &shadeCode,
			ShadeName:    &shadeName,
			CrossSection: &crossSection,
			LDRType:      mbspin.LDRTypeCalculated,
			LDRIsActual:  false,
		})

		p := mbSpinEntityToProto(e)

		assert.Equal(t, shadeCode, p.MbsShadeCode)
		assert.Equal(t, shadeName, p.MbsShadeName)
		assert.Equal(t, crossSection, p.MbsCrossSection)
	})

	t.Run("nil values map to empty string, not panic", func(t *testing.T) {
		e := newSpinWithFlags(t, nil, nil)

		p := mbSpinEntityToProto(e)

		assert.Empty(t, p.MbsShadeCode)
		assert.Empty(t, p.MbsShadeName)
		assert.Empty(t, p.MbsCrossSection)
	})
}

// captureRepo is a minimal mbspin.Repository that records the entity handed to
// Create/Update so the request-side wiring can be asserted without a database.
type captureRepo struct {
	created *mbspin.Entity
	stored  *mbspin.Entity
	updated *mbspin.Entity
}

func (r *captureRepo) Create(_ context.Context, e *mbspin.Entity) error { r.created = e; return nil }
func (r *captureRepo) GetByID(_ context.Context, _ uuid.UUID) (*mbspin.Entity, error) {
	return r.stored, nil
}

func (r *captureRepo) List(_ context.Context, _ mbspin.ListFilter) ([]*mbspin.Entity, int64, error) {
	return nil, 0, nil
}
func (r *captureRepo) Update(_ context.Context, e *mbspin.Entity) error { r.updated = e; return nil }
func (r *captureRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (r *captureRepo) ExistsByID(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }
func (r *captureRepo) GetByMBCosting(_ context.Context, _ string) (*mbspin.Entity, error) {
	return nil, nil //nolint:nilnil // test double: "not found" is expressed as nil entity
}

func (r *captureRepo) GetByOrionItemCode(_ context.Context, _ string) (*mbspin.Entity, error) {
	return nil, nil //nolint:nilnil // test double: "not found" is expressed as nil entity
}

// P8 duplicate/lineage primitives — inert stubs so captureRepo still satisfies
// the widened mbspin.Repository interface. The fixed-flag wiring under test does
// not touch the duplicate path.
func (r *captureRepo) DuplicateSpin(_ context.Context, _ mbspin.DuplicateInput) (mbspin.DuplicateOutput, error) {
	return mbspin.DuplicateOutput{}, nil
}

func (r *captureRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]*mbspin.Entity, error) {
	return nil, nil
}

func (r *captureRepo) ExistsByOrionItemCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (r *captureRepo) ResolveUniqueByOrionItemCode(_ context.Context, _ string) (uuid.UUID, bool, error) {
	return uuid.UUID{}, false, nil
}

func (r *captureRepo) ListByOrionItemCode(_ context.Context, _ string) ([]*mbspin.Entity, error) {
	return nil, nil
}

func (r *captureRepo) HasChildren(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (r *captureRepo) IsUsedByCostProduct(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

// TestCreateMBSpin_FixedFlagsWiring proves the create request path carries the
// markers through to the domain, and that an absent marker stays nil (NULL) rather
// than being defaulted to false.
func TestCreateMBSpin_FixedFlagsWiring(t *testing.T) {
	no := false

	t.Run("absent markers stay nil", func(t *testing.T) {
		repo := &captureRepo{}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		resp, err := h.CreateMBSpin(context.Background(), &financev1.CreateMBSpinRequest{
			MbhId:      uuid.New().String(),
			MbsMgtName: "SPIN-A",
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.created)
		assert.Nil(t, repo.created.LDRIsFixed(), "absent request marker must persist as NULL")
		assert.Nil(t, repo.created.DozingIsFixed(), "absent request marker must persist as NULL")
		assert.Nil(t, resp.Data.MbsLdrIsFixed)
	})

	t.Run("explicit false is carried, not dropped", func(t *testing.T) {
		repo := &captureRepo{}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		resp, err := h.CreateMBSpin(context.Background(), &financev1.CreateMBSpinRequest{
			MbhId:            uuid.New().String(),
			MbsMgtName:       "SPIN-B",
			MbsLdrIsFixed:    &no,
			MbsDozingIsFixed: &no,
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.created)
		require.NotNil(t, repo.created.LDRIsFixed(), "false marker must reach the domain")
		assert.False(t, *repo.created.LDRIsFixed())
		assert.False(t, repo.created.IsFixedLDR(), "explicit false means recalculable")
		require.NotNil(t, repo.created.DozingIsFixed())
		assert.False(t, repo.created.IsFixedDozing())
		require.NotNil(t, resp.Data.MbsDozingIsFixed)
		assert.False(t, *resp.Data.MbsDozingIsFixed)
	})
}

// TestUpdateMBSpin_FixedFlagsWiring proves the update path forwards the markers and
// that omitting them leaves the stored marker untouched (absent != false).
func TestUpdateMBSpin_FixedFlagsWiring(t *testing.T) {
	yes, no := true, false

	t.Run("explicit false flips a fixed row to recalculable", func(t *testing.T) {
		repo := &captureRepo{stored: newSpinWithFlags(t, &yes, &yes)}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
			MbhId:            uuid.New().String(),
			MbsId:            uuid.New().String(),
			MbsLdrIsFixed:    &no,
			MbsDozingIsFixed: &no,
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.updated)
		assert.False(t, repo.updated.IsFixedLDR())
		assert.False(t, repo.updated.IsFixedDozing())
	})

	t.Run("omitted markers leave the stored value untouched", func(t *testing.T) {
		repo := &captureRepo{stored: newSpinWithFlags(t, &no, &no)}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
			MbhId: uuid.New().String(),
			MbsId: uuid.New().String(),
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.updated)
		require.NotNil(t, repo.updated.LDRIsFixed())
		assert.False(t, *repo.updated.LDRIsFixed(), "omitting the marker must not reset it")
	})
}
