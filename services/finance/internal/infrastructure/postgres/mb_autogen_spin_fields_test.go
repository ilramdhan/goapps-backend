// Package postgres — unit coverage for mbBuildAutoGenSpin (the pure, DB-free construction step
// factored out of mbAutoGenSpin, mb_autogen_repository.go) proving P2-T5's field derivation:
// mbs_cc, mbs_final_product, mbs_ldr_calculated_pct and mbs_lusture_code are copied down from
// the parent MB Head rather than left nil, while shade/cross-section/denier/filament/dozing/
// mb-costing/vs-number stay wired exactly as before. No database connection is needed — this
// only exercises entity construction, not the createSpinOn insert.
package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func mbAutoGenSpinFieldsHeadParams(overrides func(*mbhead.NewParams)) mbhead.NewParams {
	p := mbhead.NewParams{
		MBCosting:    "MBC-001",
		CreatedBy:    "tester",
		Denier:       floatPtr(150.5),
		Filament:     intPtr(48),
		Dozing:       floatPtr(2.5),
		ShadeCode:    "SH-001",
		ShadeName:    "JET BLACK",
		CrossSection: "ROUND",
		LustureCode:  "LU01",
		VSNumber:     stringPtr("VS-9"),
	}
	if overrides != nil {
		overrides(&p)
	}
	return p
}

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }
func stringPtr(v string) *string  { return &v }

// A fully-populated MB Head must have every P2-T5 field carried onto the auto-generated MB
// Spin: mbs_cc from ShadeCode, mbs_final_product from MBHFinalProduct, mbs_ldr_calculated_pct
// preferring MBHRunLdrPct over MBHLdrPrsn, and mbs_lusture_code from LustureCode.
func TestMBBuildAutoGenSpin_FullyPopulatedHead_CopiesAllFields(t *testing.T) {
	params := mbAutoGenSpinFieldsHeadParams(func(p *mbhead.NewParams) {
		p.MBHFinalProduct = stringPtr("FP-77")
		p.MBHRunLdrPct = floatPtr(12.34)
		p.MBHLdrPrsn = floatPtr(99.99) // must be ignored: RunLdrPct takes priority
	})
	head, err := mbhead.New(params)
	require.NoError(t, err)

	spin, err := mbBuildAutoGenSpin(uuid.New(), head, 4242, "tester")
	require.NoError(t, err)

	require.NotNil(t, spin.CC())
	require.Equal(t, "SH-001", *spin.CC())

	require.NotNil(t, spin.MBSFinalProduct())
	require.Equal(t, "FP-77", *spin.MBSFinalProduct())

	require.NotNil(t, spin.LDRCalculatedPct())
	require.InDelta(t, 12.34, *spin.LDRCalculatedPct(), 0.0001)

	require.NotNil(t, spin.LustureCode())
	require.Equal(t, "LU01", *spin.LustureCode())

	// Verified-not-changed fields (already wired before this task).
	require.NotNil(t, spin.ShadeCode())
	require.Equal(t, "SH-001", *spin.ShadeCode())
	require.NotNil(t, spin.ShadeName())
	require.Equal(t, "JET BLACK", *spin.ShadeName())
	require.NotNil(t, spin.CrossSection())
	require.Equal(t, "ROUND", *spin.CrossSection())
	require.NotNil(t, spin.Denier())
	require.InDelta(t, 150.5, *spin.Denier(), 0.0001)
	require.NotNil(t, spin.Filament())
	require.Equal(t, 48, *spin.Filament())
	require.NotNil(t, spin.Dozing())
	require.InDelta(t, 2.5, *spin.Dozing(), 0.0001)
	require.NotNil(t, spin.MBCosting())
	require.Equal(t, "MBC-001", *spin.MBCosting())
	require.NotNil(t, spin.VSNumber())
	require.Equal(t, "VS-9", *spin.VSNumber())

	// Untouched by this task — must stay nil/false/fixed defaults.
	require.Nil(t, spin.OracleSysID())
	require.Nil(t, spin.OrionItemCode())
	require.Nil(t, spin.CostRateMkt())
	require.Nil(t, spin.MBSLdrPrsn())
	require.Nil(t, spin.MBSRunLdrPct())
	require.Nil(t, spin.LDRIsFixed())
	require.Nil(t, spin.DozingIsFixed())
	require.Equal(t, mbspin.LDRTypeNotCalculated, spin.LDRType())
	require.False(t, spin.LDRIsActual())
	require.Nil(t, spin.LDRAdjustmentPct())
	require.NotNil(t, spin.MBSStatus())
	require.Equal(t, mbspin.StatusRnD, *spin.MBSStatus())
}

// When the head's RunLdrPct is nil, mbs_ldr_calculated_pct must fall back to the head's
// MBHLdrPrsn instead.
func TestMBBuildAutoGenSpin_NoRunLdrPct_FallsBackToLdrPrsn(t *testing.T) {
	params := mbAutoGenSpinFieldsHeadParams(func(p *mbhead.NewParams) {
		p.MBHRunLdrPct = nil
		p.MBHLdrPrsn = floatPtr(55.5)
	})
	head, err := mbhead.New(params)
	require.NoError(t, err)

	spin, err := mbBuildAutoGenSpin(uuid.New(), head, 1, "tester")
	require.NoError(t, err)

	require.NotNil(t, spin.LDRCalculatedPct())
	require.InDelta(t, 55.5, *spin.LDRCalculatedPct(), 0.0001)
}

// A MB Head with empty/nil optional fields (final product, both LDR sources, lusture code)
// must produce NULL (nil pointers) on the MB Spin — never an empty string or a zero float
// masquerading as a real value.
func TestMBBuildAutoGenSpin_EmptyOptionalFields_LeaveSpinFieldsNil(t *testing.T) {
	params := mbAutoGenSpinFieldsHeadParams(func(p *mbhead.NewParams) {
		p.MBHFinalProduct = nil
		p.MBHRunLdrPct = nil
		p.MBHLdrPrsn = nil
		p.LustureCode = ""
		p.ShadeCode = ""
	})
	head, err := mbhead.New(params)
	require.NoError(t, err)

	spin, err := mbBuildAutoGenSpin(uuid.New(), head, 1, "tester")
	require.NoError(t, err)

	require.Nil(t, spin.MBSFinalProduct())
	require.Nil(t, spin.LDRCalculatedPct())
	require.Nil(t, spin.LustureCode())
	require.Nil(t, spin.CC(), "mbs_cc must be NULL, not empty string, when head ShadeCode is empty")
}

// D3's fallback (MBHRunLdrPct -> MBHLdrPrsn) checks the pointer for nil, not the pointed-to
// value. A head whose MBHRunLdrPct is an explicit, valid 0.0 (e.g. a freshly recalculated LDR of
// exactly zero) must come out as 0.0 on the spin, never silently replaced by MBHLdrPrsn's
// non-zero value — "unset" and "computed to zero" are different meanings and must not collapse
// into each other. TestMBBuildAutoGenSpin_NoRunLdrPct_FallsBackToLdrPrsn already covers the
// nil case; this covers the zero-but-present case the other three tests do not exercise.
func TestMBBuildAutoGenSpin_RunLdrPctIsZero_UsesZeroNotFallback(t *testing.T) {
	params := mbAutoGenSpinFieldsHeadParams(func(p *mbhead.NewParams) {
		p.MBHRunLdrPct = floatPtr(0)
		p.MBHLdrPrsn = floatPtr(77.7) // must be ignored: an explicit 0.0 RunLdrPct still wins
	})
	head, err := mbhead.New(params)
	require.NoError(t, err)

	spin, err := mbBuildAutoGenSpin(uuid.New(), head, 1, "tester")
	require.NoError(t, err)

	require.NotNil(t, spin.LDRCalculatedPct(), "an explicit 0.0 must not be treated as absent")
	require.InDelta(t, 0.0, *spin.LDRCalculatedPct(), 0.0001)
}

// The fields that must NEVER be populated by auto-gen (Oracle/ORION identifiers, cost-rate-mkt,
// the raw Oracle LDR columns, and the two "is fixed" flags) must stay nil regardless of which MB
// Head shape produced the spin — TestMBBuildAutoGenSpin_FullyPopulatedHead_CopiesAllFields only
// proves this for ONE (fully-populated) head, which leaves open whether it is incidentally true
// only for that shape. This exercises the same "always nil" assertions against a second,
// differently-shaped (nearly empty) head to close that gap.
func TestMBBuildAutoGenSpin_AlwaysNilFields_StayNilForMinimalHead(t *testing.T) {
	params := mbAutoGenSpinFieldsHeadParams(func(p *mbhead.NewParams) {
		p.Denier = nil
		p.Filament = nil
		p.Dozing = nil
		p.MBHFinalProduct = nil
		p.MBHRunLdrPct = nil
		p.MBHLdrPrsn = nil
		p.LustureCode = ""
		p.ShadeCode = ""
		p.ShadeName = ""
		p.CrossSection = ""
		p.VSNumber = nil
	})
	head, err := mbhead.New(params)
	require.NoError(t, err)

	spin, err := mbBuildAutoGenSpin(uuid.New(), head, 999, "tester")
	require.NoError(t, err)

	require.Nil(t, spin.OracleSysID())
	require.Nil(t, spin.OrionItemCode())
	require.Nil(t, spin.CostRateMkt())
	require.Nil(t, spin.MBSLdrPrsn())
	require.Nil(t, spin.MBSRunLdrPct())
	require.Nil(t, spin.LDRIsFixed())
	require.Nil(t, spin.DozingIsFixed())
	require.Equal(t, mbspin.LDRTypeNotCalculated, spin.LDRType())
	require.False(t, spin.LDRIsActual())
	require.Nil(t, spin.LDRAdjustmentPct())
}
