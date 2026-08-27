package mbhead_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// fullRecipeParams returns NewParams with every field the required-field rule set
// checks already populated, so each test can blank out exactly one of them.
func fullRecipeParams() mbhead.NewParams {
	name := "MGT NAME"
	denier := 150.0
	filament := 48
	ldr := 2.5
	finalProduct := "FP-1"
	vs := "16728"
	return mbhead.NewParams{
		MBCosting:       "MBH-2024-001",
		CreatedBy:       "admin",
		MgtName:         &name,
		Denier:          &denier,
		Filament:        &filament,
		MBHLdrPrsn:      &ldr,
		MBHFinalProduct: &finalProduct,
		VSNumber:        &vs,
		DevCode:         "DEV-1",
		ShadeCode:       "SH1",
		ShadeName:       "Red",
		CrossSection:    "RND",
	}
}

func TestValidateRecipeFields_AllPresent_NoError(t *testing.T) {
	e, err := mbhead.New(fullRecipeParams())
	require.NoError(t, err)
	require.NoError(t, e.ValidateRecipeFields())
}

// TestValidateRecipeFields_RuleSetSize pins the rule count at TEN.
//
// TODO(U-A): the design and plan text both say "11 required fields", but the
// user's enumerated list names only ten. The eleventh is not written down
// anywhere and is ⛔ not guessed at here. If the user supplies it, this number and
// the rule set both change together.
func TestValidateRecipeFields_RuleSetSize(t *testing.T) {
	assert.Len(t, mbhead.RequiredRecipeFields(), 10)
}

func TestValidateRecipeFields_EachFieldMissing_ReturnsErrRecipeFieldRequired(t *testing.T) {
	blank := map[string]func(p *mbhead.NewParams){
		"mbh_mgt_name":      func(p *mbhead.NewParams) { p.MgtName = nil },
		"mbh_dev_code":      func(p *mbhead.NewParams) { p.DevCode = "" },
		"mbh_vs_number":     func(p *mbhead.NewParams) { p.VSNumber = nil },
		"mbh_shade_code":    func(p *mbhead.NewParams) { p.ShadeCode = "" },
		"mbh_shade_name":    func(p *mbhead.NewParams) { p.ShadeName = "" },
		"mbh_denier":        func(p *mbhead.NewParams) { p.Denier = nil },
		"mbh_filament":      func(p *mbhead.NewParams) { p.Filament = nil },
		"mbh_cross_section": func(p *mbhead.NewParams) { p.CrossSection = "" },
		"mbh_ldr_prsn":      func(p *mbhead.NewParams) { p.MBHLdrPrsn = nil },
		"mbh_final_product": func(p *mbhead.NewParams) { p.MBHFinalProduct = nil },
	}
	require.Len(t, blank, len(mbhead.RequiredRecipeFields()),
		"every required field must have a blanking case")

	for column, clear := range blank {
		t.Run(column, func(t *testing.T) {
			p := fullRecipeParams()
			clear(&p)
			e, err := mbhead.New(p)
			require.NoError(t, err)

			err = e.ValidateRecipeFields()
			require.ErrorIs(t, err, mbhead.ErrRecipeFieldRequired)
			assert.Contains(t, err.Error(), column,
				"error must name the offending column")
		})
	}
}

// TestValidateRecipeFields_VSNumberFreeText documents B9/OQ-17: VS Number carries
// no format rule, so the literal "NA" passes.
func TestValidateRecipeFields_VSNumberFreeText(t *testing.T) {
	for _, v := range []string{"NA", "0", "  ", "16728"} {
		p := fullRecipeParams()
		vs := v
		p.VSNumber = &vs
		e, err := mbhead.New(p)
		require.NoError(t, err)
		assert.NoErrorf(t, e.ValidateRecipeFields(), "vs number %q must pass", v)
	}
}

// TestReconstruct_LegacyRow_NotValidated pins K8: an entity rebuilt from legacy
// storage is never auto-validated. Loading must succeed even though the row is
// missing required fields; only an explicit ValidateRecipeFields call complains.
func TestReconstruct_LegacyRow_NotValidated(t *testing.T) {
	e := mbhead.Reconstruct(
		uuidNil(), nil, "LEGACY-1", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		true, timeZero(), "oracle", nil, nil, nil, nil,
		"DRAFT", false, 0, nil, "", "", "", "", "", "", 0, nil, "",
		nil, nil, nil, nil, nil, nil, "", "", nil,
	)
	require.NotNil(t, e)
	assert.Equal(t, "LEGACY-1", e.MBCosting())
	require.ErrorIs(t, e.ValidateRecipeFields(), mbhead.ErrRecipeFieldRequired)
}

func uuidNil() uuid.UUID { return uuid.Nil }

func timeZero() time.Time { return time.Time{} }
