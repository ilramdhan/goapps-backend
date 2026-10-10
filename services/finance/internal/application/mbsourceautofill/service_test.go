package mbsourceautofill_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

type fakeStore struct {
	targets  []mbsourceautofill.Target
	listErr  error
	applyErr map[int64]error
	applied  []mbsourceautofill.Plan
}

func (f *fakeStore) ListTargets(context.Context, []int64) ([]mbsourceautofill.Target, error) {
	return f.targets, f.listErr
}
func (f *fakeStore) Apply(_ context.Context, _ mbsourceautofill.ParamSet, p mbsourceautofill.Plan, _ string) error {
	if e := f.applyErr[p.ProductSysID]; e != nil {
		return e
	}
	f.applied = append(f.applied, p)
	return nil
}

type fakeResolver struct {
	res map[string]mbsource.Resolution
	err error
}

func (f fakeResolver) Resolve(context.Context, []string) (map[string]mbsource.Resolution, error) {
	return f.res, f.err
}

// params is built via the parameter package constructor path would need DB; the service only
// needs ids, so use a reader that returns reconstructed params.
type fakeParams struct{ err error }

func (f fakeParams) GetByCode(context.Context, parameter.Code) (*parameter.Parameter, error) {
	return nil, f.err
}
func (f fakeParams) GetByFillGroup(context.Context, string) ([]*parameter.Parameter, error) {
	return nil, nil
}

func TestAutoFill_NoMatchAndEmptyInput(t *testing.T) {
	st := &fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}}
	svc := mbsourceautofill.New(st, fakeResolver{res: map[string]mbsource.Resolution{}}, fakeParams{})
	stats, err := svc.AutoFillMBSource(context.Background(), []int64{1}, "u")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.NoMatch)
	assert.Empty(t, st.applied)

	stats, err = svc.AutoFillMBSource(context.Background(), nil, "u")
	require.NoError(t, err)
	assert.Zero(t, stats.Considered)
}

func TestAutoFill_NoTargets_SkipsResolver(t *testing.T) {
	svc := mbsourceautofill.New(&fakeStore{}, fakeResolver{err: errors.New("must not be called")}, fakeParams{})
	_, err := svc.AutoFillMBSource(context.Background(), []int64{1}, "u")
	require.NoError(t, err)
}

func TestBestEffort_NeverPropagates(t *testing.T) {
	// store error
	svc := mbsourceautofill.New(&fakeStore{listErr: errors.New("db")}, fakeResolver{}, fakeParams{})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
	// resolver error
	svc = mbsourceautofill.New(&fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}},
		fakeResolver{err: errors.New("boom")}, fakeParams{})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
	// param lookup error after a hit
	svc = mbsourceautofill.New(&fakeStore{targets: []mbsourceautofill.Target{{ProductSysID: 1, Shade: "x"}}},
		fakeResolver{res: map[string]mbsource.Resolution{"X": {SPCode: "X"}}}, fakeParams{err: errors.New("no param")})
	assert.NotPanics(t, func() { svc.BestEffort(context.Background(), []int64{1}, "u") })
}
