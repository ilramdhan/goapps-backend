package mbspin_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestNew_Success(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Equal(t, headID, e.HeadID())
	assert.Equal(t, "MB Spin A", e.MgtName())
	assert.True(t, e.IsActive())
	assert.Nil(t, e.CC())
	assert.Nil(t, e.CostRateMkt())
}

func TestNew_InvalidHeadID(t *testing.T) {
	_, err := mbspin.New(uuid.Nil, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrInvalidHeadID)
}

func TestNew_EmptyMgtName(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrEmptyMgtName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	assert.ErrorIs(t, err, mbspin.ErrEmptyCreatedBy)
}

func TestUpdate_Success(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Old Name", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	newName := "New Name"
	err = e.Update(mbspin.UpdateInput{MgtName: &newName}, "editor")
	require.NoError(t, err)
	assert.Equal(t, "New Name", e.MgtName())
}

func TestNew_WithCCAndCostRateMkt(t *testing.T) {
	headID := uuid.New()
	cc := "CC-001"
	rate := 12.5
	e, err := mbspin.New(headID, "MB Spin B", nil, nil, nil, nil, nil, nil, &cc, &rate, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NotNil(t, e.CC())
	assert.Equal(t, "CC-001", *e.CC())
	require.NotNil(t, e.CostRateMkt())
	assert.InDelta(t, 12.5, *e.CostRateMkt(), 0.001)
}

func TestUpdate_CCAndCostRateMkt(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin C", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	cc := "CC-002"
	rate := 9.75
	err = e.Update(mbspin.UpdateInput{CC: &cc, CostRateMkt: &rate}, "editor")
	require.NoError(t, err)
	require.NotNil(t, e.CC())
	assert.Equal(t, "CC-002", *e.CC())
	require.NotNil(t, e.CostRateMkt())
	assert.InDelta(t, 9.75, *e.CostRateMkt(), 0.001)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Name", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), mbspin.ErrAlreadyDeleted)
}

// -----------------------------------------------------------------------------
// P12b — fix/actual markers (recalc rule #3). Migration 000486.
// -----------------------------------------------------------------------------

func newSpinWithFlags(t *testing.T, ldrFixed, dozingFixed *bool) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), "MB Spin Flags", nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, ldrFixed, dozingFixed, "admin")
	require.NoError(t, err)
	return e
}

func boolPtr(b bool) *bool { return &b }

// The four combinations recalc rule #3 must distinguish. The whole point of
// P12b is that these are INDEPENDENT — a per-row marker cannot express them.
func TestIsFixed_AllCombinations(t *testing.T) {
	tests := []struct {
		name            string
		ldrFixed        *bool
		dozingFixed     *bool
		wantLDRFixed    bool
		wantDozingFixed bool
	}{
		{"ldr actual, dozing computed", boolPtr(true), boolPtr(false), true, false},
		{"dozing actual, ldr computed", boolPtr(false), boolPtr(true), false, true},
		{"both actual", boolPtr(true), boolPtr(true), true, true},
		{"both computed", boolPtr(false), boolPtr(false), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newSpinWithFlags(t, tt.ldrFixed, tt.dozingFixed)
			assert.Equal(t, tt.wantLDRFixed, e.IsFixedLDR())
			assert.Equal(t, tt.wantDozingFixed, e.IsFixedDozing())
		})
	}
}

// nil means "unknown" and MUST be read as FIXED. All ~2699 pre-existing Oracle
// rows carry NULL; if this flipped, the first P13 recalc run would silently
// overwrite thousands of human-entered values.
func TestIsFixed_NilMeansFixed(t *testing.T) {
	e := newSpinWithFlags(t, nil, nil)
	assert.Nil(t, e.LDRIsFixed())
	assert.Nil(t, e.DozingIsFixed())
	assert.True(t, e.IsFixedLDR(), "nil LDR marker must be treated as fixed")
	assert.True(t, e.IsFixedDozing(), "nil dozing marker must be treated as fixed")
}

// Each marker is independent: one nil, one explicit.
func TestIsFixed_MixedNilAndExplicit(t *testing.T) {
	e := newSpinWithFlags(t, boolPtr(false), nil)
	assert.False(t, e.IsFixedLDR())
	assert.True(t, e.IsFixedDozing())
}

func TestUpdate_SetsFixedMarkers(t *testing.T) {
	e := newSpinWithFlags(t, nil, nil)
	require.NoError(t, e.Update(mbspin.UpdateInput{
		LDRIsFixed:    boolPtr(false),
		DozingIsFixed: boolPtr(true),
	}, "editor"))
	assert.False(t, e.IsFixedLDR())
	assert.True(t, e.IsFixedDozing())
}

// A nil field in UpdateInput must leave the stored marker untouched (D13:
// absent is not zero).
func TestUpdate_NilMarkerLeavesStoredValueUntouched(t *testing.T) {
	e := newSpinWithFlags(t, boolPtr(true), boolPtr(false))
	require.NoError(t, e.Update(mbspin.UpdateInput{}, "editor"))
	require.NotNil(t, e.LDRIsFixed())
	assert.True(t, *e.LDRIsFixed())
	require.NotNil(t, e.DozingIsFixed())
	assert.False(t, *e.DozingIsFixed())
}

func TestReconstruct_RoundTripsFixedMarkers(t *testing.T) {
	e := mbspin.Reconstruct(uuid.New(), nil, nil, uuid.New(), "MB Spin R",
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		boolPtr(true), boolPtr(false),
		true, time.Now(), "admin", nil, nil, nil, nil)
	assert.True(t, e.IsFixedLDR())
	assert.False(t, e.IsFixedDozing())
}

// =============================================================================
// LDR adjustment / lock mutators (Task E)
// =============================================================================

func newSpinForLDR(t *testing.T) *mbspin.Entity {
	t.Helper()
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin LDR", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	return e
}

func TestSetLDRAdjustment_SucceedsWhenUnlocked(t *testing.T) {
	e := newSpinForLDR(t)
	adj := 1.5
	require.NoError(t, e.SetLDRAdjustment(&adj))
	require.NotNil(t, e.LDRAdjustmentPct())
	assert.InDelta(t, 1.5, *e.LDRAdjustmentPct(), 0.001)
}

func TestSetLDRAdjustment_RejectedWhenLockedActual(t *testing.T) {
	e := newSpinForLDR(t)
	e.LockLDRActual()
	adj := 2.5
	err := e.SetLDRAdjustment(&adj)
	assert.ErrorIs(t, err, mbspin.ErrLDRLockedActual)
}

func TestSetLDRAdjustment_ClearRejectedWhenLockedActual(t *testing.T) {
	e := newSpinForLDR(t)
	e.LockLDRActual()
	err := e.SetLDRAdjustment(nil)
	assert.ErrorIs(t, err, mbspin.ErrLDRLockedActual)
}

func TestLockLDRActual_SetsBothFields(t *testing.T) {
	e := newSpinForLDR(t)
	e.LockLDRActual()
	assert.True(t, e.LDRIsActual())
	assert.Equal(t, mbspin.LDRTypeActual, e.LDRType())
}

func TestUnlockLDRActual_RevertsToCalculated_WhenCalculatedPctPresent(t *testing.T) {
	e := newSpinForLDR(t)
	calc := 3.5
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{LDRType: mbspin.LDRTypeNotCalculated, LDRCalculatedPct: &calc})
	e.LockLDRActual()
	require.True(t, e.LDRIsActual())

	e.UnlockLDRActual()
	assert.False(t, e.LDRIsActual())
	assert.Equal(t, mbspin.LDRTypeCalculated, e.LDRType())
}

func TestUnlockLDRActual_RevertsToNotCalculated_WhenCalculatedPctAbsent(t *testing.T) {
	e := newSpinForLDR(t)
	e.LockLDRActual()
	require.True(t, e.LDRIsActual())

	e.UnlockLDRActual()
	assert.False(t, e.LDRIsActual())
	assert.Equal(t, mbspin.LDRTypeNotCalculated, e.LDRType())
}

func TestUnlockLDRActual_ThenSetLDRAdjustment_Succeeds(t *testing.T) {
	e := newSpinForLDR(t)
	e.LockLDRActual()
	e.UnlockLDRActual()
	adj := 4.0
	require.NoError(t, e.SetLDRAdjustment(&adj))
	require.NotNil(t, e.LDRAdjustmentPct())
	assert.InDelta(t, 4.0, *e.LDRAdjustmentPct(), 0.001)
}

func TestNew_LustureCodeDefaultsNil(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Nil(t, e.LustureCode())
}

func TestReconstruct_LustureCodeDefaultsNil(t *testing.T) {
	e := mbspin.Reconstruct(uuid.New(), nil, nil, uuid.New(), "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, true, time.Now(), "admin", nil, nil, nil, nil)
	assert.Nil(t, e.LustureCode())
}

func TestHydrateLustureCode_SetsAndReturnsValue(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	code := "LU01"
	e.HydrateLustureCode(&code)

	require.NotNil(t, e.LustureCode())
	assert.Equal(t, "LU01", *e.LustureCode())
}

func TestHydrateLustureCode_NilClearsValue(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	code := "LU01"
	e.HydrateLustureCode(&code)
	require.NotNil(t, e.LustureCode())

	e.HydrateLustureCode(nil)
	assert.Nil(t, e.LustureCode())
}

// =============================================================================
// SetShade / SetActualLDR (MB Active Sync K1, K2)
// =============================================================================

func newSpinForSetters(t *testing.T) *mbspin.Entity {
	t.Helper()
	e, err := mbspin.New(uuid.New(), "MB Spin S", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	return e
}

func TestSetShade_SetsShadeAndMirrorsCC(t *testing.T) {
	e := newSpinForSetters(t)
	code, name := "SH01", "Shade One"
	require.NoError(t, e.SetShade(&code, &name, nil, "sync"))
	require.NotNil(t, e.ShadeCode())
	assert.Equal(t, "SH01", *e.ShadeCode())
	require.NotNil(t, e.ShadeName())
	assert.Equal(t, "Shade One", *e.ShadeName())
	require.NotNil(t, e.CC())
	assert.Equal(t, "SH01", *e.CC())
	require.NotNil(t, e.UpdatedBy())
	assert.Equal(t, "sync", *e.UpdatedBy())
	assert.NotNil(t, e.UpdatedAt())
}

func TestSetShade_NilClearsCodeAndCC(t *testing.T) {
	e := newSpinForSetters(t)
	code := "SH01"
	require.NoError(t, e.SetShade(&code, nil, nil, "sync"))
	require.NoError(t, e.SetShade(nil, nil, nil, "sync"))
	assert.Nil(t, e.ShadeCode())
	assert.Nil(t, e.ShadeName())
	assert.Nil(t, e.CC())
}

func TestSetShade_SetsCrossSection(t *testing.T) {
	e := newSpinForSetters(t)
	cs := "CS-B"
	code := "SH02"
	require.NoError(t, e.SetShade(&code, nil, &cs, "sync"))
	require.NotNil(t, e.CrossSection())
	assert.Equal(t, "CS-B", *e.CrossSection())
}

func TestSetShade_KeepsCrossSectionWhenPassedCurrent(t *testing.T) {
	e := newSpinForSetters(t)
	cs := "CS-A"
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{CrossSection: &cs, LDRType: mbspin.LDRTypeNotCalculated})
	code := "SH02"
	require.NoError(t, e.SetShade(&code, nil, e.CrossSection(), "sync"))
	require.NotNil(t, e.CrossSection())
	assert.Equal(t, "CS-A", *e.CrossSection())
}

func TestSetShade_NilClearsCrossSection(t *testing.T) {
	e := newSpinForSetters(t)
	cs := "CS-A"
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{CrossSection: &cs, LDRType: mbspin.LDRTypeNotCalculated})
	require.NoError(t, e.SetShade(nil, nil, nil, "sync"))
	assert.Nil(t, e.CrossSection())
}

func TestSetShade_RejectedWhenDeleted(t *testing.T) {
	e := newSpinForSetters(t)
	require.NoError(t, e.SoftDelete("admin"))
	code := "SH01"
	assert.ErrorIs(t, e.SetShade(&code, nil, nil, "sync"), mbspin.ErrAlreadyDeleted)
	assert.Nil(t, e.ShadeCode())
}

func TestSetActualLDR_SetsPctFixedAndLocksActual(t *testing.T) {
	e := newSpinForSetters(t)
	require.NoError(t, e.SetActualLDR(12.5, "sync"))
	require.NotNil(t, e.MBSRunLdrPct())
	assert.InDelta(t, 12.5, *e.MBSRunLdrPct(), 1e-9)
	require.NotNil(t, e.LDRIsFixed())
	assert.True(t, *e.LDRIsFixed())
	assert.True(t, e.LDRIsActual())
	assert.Equal(t, mbspin.LDRTypeActual, e.LDRType())
	require.NotNil(t, e.UpdatedBy())
	assert.Equal(t, "sync", *e.UpdatedBy())
}

func TestSetActualLDR_RejectsNonPositive(t *testing.T) {
	for _, pct := range []float64{0, -1} {
		e := newSpinForSetters(t)
		assert.ErrorIs(t, e.SetActualLDR(pct, "sync"), mbspin.ErrInvalidActualLDR)
		assert.Nil(t, e.MBSRunLdrPct())
		assert.False(t, e.LDRIsActual())
	}
}

func TestSetActualLDR_RejectedWhenDeleted(t *testing.T) {
	e := newSpinForSetters(t)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SetActualLDR(5, "sync"), mbspin.ErrAlreadyDeleted)
	assert.False(t, e.LDRIsActual())
}

func TestSetActualLDR_BlocksAdjustmentUntilUnlocked(t *testing.T) {
	e := newSpinForSetters(t)
	require.NoError(t, e.SetActualLDR(7, "sync"))
	adj := 1.0
	assert.ErrorIs(t, e.SetLDRAdjustment(&adj), mbspin.ErrLDRLockedActual)
	e.UnlockLDRActual()
	require.NoError(t, e.SetLDRAdjustment(&adj))
}
