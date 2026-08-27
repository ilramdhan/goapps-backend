package mbdozing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

// fakeFactorRepo serves a fixed set of ordered pairs. Any pair not in the map
// returns ErrFactorNotFound, mirroring the real repository. No database is used
// anywhere in this file.
type fakeFactorRepo struct {
	pairs map[string]*mbcrosssection.FactorEntity
	calls int
}

func (f *fakeFactorRepo) GetByPair(_ context.Context, fromCode, toCode string) (*mbcrosssection.FactorEntity, error) {
	f.calls++
	if e, ok := f.pairs[fromCode+"->"+toCode]; ok {
		return e, nil
	}
	return nil, mbcrosssection.ErrFactorNotFound
}

func (f *fakeFactorRepo) Create(context.Context, *mbcrosssection.FactorEntity) error { return nil }
func (f *fakeFactorRepo) Update(context.Context, *mbcrosssection.FactorEntity) error { return nil }
func (f *fakeFactorRepo) Delete(context.Context, string, string) error               { return nil }
func (f *fakeFactorRepo) GetByID(context.Context, string) (*mbcrosssection.FactorEntity, error) {
	return nil, mbcrosssection.ErrFactorNotFound
}
func (f *fakeFactorRepo) List(context.Context, mbcrosssection.FactorListFilter) ([]*mbcrosssection.FactorEntity, int64, error) {
	return nil, 0, nil
}

var _ mbcrosssection.FactorRepository = (*fakeFactorRepo)(nil)

func rndToTbl() *mbcrosssection.FactorEntity {
	return mbcrosssection.ReconstructFactor(
		"factor-1", "RND", "TBL", 1.15, mbcrosssection.OperationMultiply,
		"", true, "", "seed", "", "", "", "")
}

// newHandler builds a handler with no formula repository, so formula_code falls
// back to the canonical constant. A nil formula repo is never dereferenced.
func newHandler(repo mbcrosssection.FactorRepository) *app.CalculateHandler {
	return app.NewCalculateHandler(repo, nil)
}

func f64(v float64) *float64 { return &v }
func i32(v int32) *int32     { return &v }

// TestXSectionMissingFactorIsNormalPath is the constraint-D13 guard: a pair with
// no factor row must come back as a successful, factor_available = false result
// with NO LDR — never an error, and never a substituted neutral factor.
func TestXSectionMissingFactorIsNormalPath(t *testing.T) {
	repo := &fakeFactorRepo{pairs: map[string]*mbcrosssection.FactorEntity{
		"RND->TBL": rndToTbl(),
	}}

	res, err := newHandler(repo).Handle(context.Background(), app.CalculateCommand{
		Mode:             mbdozing.ModeXSection,
		LDRSource:        f64(0.9),
		FromCrossSection: strPtr("RND"),
		ToCrossSection:   strPtr("OTL"),
	})

	require.NoError(t, err, "an unsupported pair is a normal outcome, not an error")
	require.NotNil(t, res)
	assert.False(t, res.FactorAvailable)
	assert.Nil(t, res.ResultLDR, "no factor means no number; substituting a neutral factor is forbidden")
	assert.NotEmpty(t, res.Message)
	assert.Contains(t, res.Message, "RND")
	assert.Contains(t, res.Message, "OTL")
	assert.Equal(t, app.FormulaCodeXSection, res.FormulaCode)
	assert.Empty(t, res.CalculationTrace)
}

func TestXSectionWithFactorMultiplies(t *testing.T) {
	repo := &fakeFactorRepo{pairs: map[string]*mbcrosssection.FactorEntity{
		"RND->TBL": rndToTbl(),
	}}

	res, err := newHandler(repo).Handle(context.Background(), app.CalculateCommand{
		Mode:             mbdozing.ModeXSection,
		LDRSource:        f64(0.9),
		FromCrossSection: strPtr("RND"),
		ToCrossSection:   strPtr("TBL"),
	})

	require.NoError(t, err)
	require.NotNil(t, res.ResultLDR)
	assert.True(t, res.FactorAvailable)
	assert.InDelta(t, 0.9*1.15, *res.ResultLDR, 1e-12)
	assert.Contains(t, res.CalculationTrace, "1.15")
	assert.Contains(t, res.CalculationTrace, "*")
}

func TestXSectionDivideUsesMasterOperation(t *testing.T) {
	div := mbcrosssection.ReconstructFactor(
		"factor-2", "TBL", "RND", 1.15, mbcrosssection.OperationDivide,
		"", true, "", "seed", "", "", "", "")
	repo := &fakeFactorRepo{pairs: map[string]*mbcrosssection.FactorEntity{"TBL->RND": div}}

	res, err := newHandler(repo).Handle(context.Background(), app.CalculateCommand{
		Mode:             mbdozing.ModeXSection,
		LDRSource:        f64(1.035),
		FromCrossSection: strPtr("TBL"),
		ToCrossSection:   strPtr("RND"),
	})

	require.NoError(t, err)
	require.NotNil(t, res.ResultLDR)
	assert.InDelta(t, 1.035/1.15, *res.ResultLDR, 1e-12)
	assert.Contains(t, res.CalculationTrace, "/")
}

// TestScaleGoldenCase pins the C-1 result and proves the trace is built by string
// formatting, not by evaluating an expression.
func TestScaleGoldenCase(t *testing.T) {
	repo := &fakeFactorRepo{pairs: map[string]*mbcrosssection.FactorEntity{}}

	res, err := newHandler(repo).Handle(context.Background(), app.CalculateCommand{
		Mode:           mbdozing.ModeScale,
		LDRRef:         f64(0.9),
		DenierRef:      f64(380),
		FilamentRef:    i32(108),
		DenierTarget:   f64(500),
		FilamentTarget: i32(96),
	})

	require.NoError(t, err)
	require.NotNil(t, res.ResultLDR)
	assert.True(t, res.FactorAvailable)
	assert.InDelta(t, 0.7397296803562773, *res.ResultLDR, 1e-12)
	assert.Equal(t, app.FormulaCodeScale, res.FormulaCode)
	assert.Contains(t, res.CalculationTrace, "sqrt(380 / 108)")
	assert.Equal(t, 0, repo.calls, "SCALE must not consult the factor repository")
}

func TestScaleZeroFilamentIsRejected(t *testing.T) {
	res, err := newHandler(&fakeFactorRepo{}).Handle(context.Background(), app.CalculateCommand{
		Mode:           mbdozing.ModeScale,
		LDRRef:         f64(0.9),
		DenierRef:      f64(380),
		FilamentRef:    i32(0),
		DenierTarget:   f64(500),
		FilamentTarget: i32(96),
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, mbdozing.ErrZeroFilament))
	assert.Nil(t, res)
}

func TestScaleMissingOperandIsRejected(t *testing.T) {
	_, err := newHandler(&fakeFactorRepo{}).Handle(context.Background(), app.CalculateCommand{
		Mode:      mbdozing.ModeScale,
		LDRRef:    f64(0.9),
		DenierRef: f64(380),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, app.ErrMissingScaleInput))
}

func TestXSectionMissingOperandIsRejected(t *testing.T) {
	_, err := newHandler(&fakeFactorRepo{}).Handle(context.Background(), app.CalculateCommand{
		Mode:      mbdozing.ModeXSection,
		LDRSource: f64(0.9),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, app.ErrMissingXSectionInput))
}

// TestUnknownModeIsRejected guards the withheld third mode (gate G6-C3): modes
// other than SCALE and XSECTION are deliberately unimplemented and must be
// refused rather than guessed at.
func TestUnknownModeIsRejected(t *testing.T) {
	for _, mode := range []string{"", "scale", "SOMETHING_ELSE"} {
		_, err := newHandler(&fakeFactorRepo{}).Handle(context.Background(), app.CalculateCommand{Mode: mode})
		require.Error(t, err, "mode %q", mode)
		assert.True(t, errors.Is(err, mbdozing.ErrInvalidMode), "mode %q", mode)
	}
}

func strPtr(s string) *string { return &s }
