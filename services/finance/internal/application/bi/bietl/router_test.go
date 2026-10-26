package bietl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/bi/bietl"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/datasource"
)

type lookupStub struct {
	byID map[uuid.UUID]string
	err  error
}

func (l lookupStub) GetByID(_ context.Context, id uuid.UUID) (*datasource.DataSource, error) {
	if l.err != nil {
		return nil, l.err
	}
	code, ok := l.byID[id]
	if !ok {
		return nil, datasource.ErrNotFound
	}
	return &datasource.DataSource{ID: id, Code: code}, nil
}

type runnerStub struct {
	calls  int
	gotTyp string
	rows   int
}

func (r *runnerStub) Load(_ context.Context, targetType, _ string, _ uuid.UUID) (int, error) {
	r.calls++
	r.gotTyp = targetType
	return r.rows, nil
}

func TestRouter_DispatchesBySourceCode(t *testing.T) {
	erpID, hrisID := uuid.New(), uuid.New()
	lookup := lookupStub{byID: map[uuid.UUID]string{erpID: bietl.SourceERPOracle, hrisID: bietl.SourceHRISOracle}}
	erp, hris := &runnerStub{rows: 1}, &runnerStub{rows: 2}

	r := bietl.NewRouter(lookup)
	r.Register(bietl.SourceERPOracle, erp)
	r.Register(bietl.SourceHRISOracle, hris)

	n, err := r.Load(context.Background(), "MIS", "V", erpID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = r.Load(context.Background(), "HR_OT_HOURS", "V", hrisID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 1, erp.calls)
	assert.Equal(t, 1, hris.calls)
	assert.Equal(t, "HR_OT_HOURS", hris.gotTyp)
}

// HD-02: HRIS env not set ⇒ job fails with a clear message and must NOT fall back to ERP.
func TestRouter_UnregisteredSourceFailsClearly(t *testing.T) {
	erpID, hrisID := uuid.New(), uuid.New()
	lookup := lookupStub{byID: map[uuid.UUID]string{erpID: bietl.SourceERPOracle, hrisID: bietl.SourceHRISOracle}}
	erp := &runnerStub{}

	r := bietl.NewRouter(lookup)
	r.Register(bietl.SourceERPOracle, erp)
	r.Register(bietl.SourceHRISOracle, nil) // nil runner = not installed

	_, err := r.Load(context.Background(), "HR_OT_HOURS", "V", hrisID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), bietl.SourceHRISOracle)
	assert.Contains(t, err.Error(), "not configured")
	assert.Zero(t, erp.calls, "tidak boleh jatuh ke runner ERP")
}

func TestRouter_LookupErrorIsWrapped(t *testing.T) {
	r := bietl.NewRouter(lookupStub{err: datasource.ErrNotFound})
	_, err := r.Load(context.Background(), "X", "V", uuid.New())
	assert.True(t, errors.Is(err, datasource.ErrNotFound))
}

// PP-03: pasangan source + target_type didahulukan, lalu jatuh ke runner per source.
func TestRouter_TargetLookupThenSourceFallback(t *testing.T) {
	hrisID := uuid.New()
	lookup := lookupStub{byID: map[uuid.UUID]string{hrisID: bietl.SourceHRISOracle}}
	hris, pack := &runnerStub{rows: 2}, &runnerStub{rows: 7}

	r := bietl.NewRouter(lookup)
	r.Register(bietl.SourceHRISOracle, hris)
	r.RegisterTarget(bietl.SourceHRISOracle, "PP_PACK", pack)
	r.RegisterTarget(bietl.SourceHRISOracle, "PP_NIL", nil) // nil runner diabaikan

	n, err := r.Load(context.Background(), "PP_PACK", "V", hrisID)
	require.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.Equal(t, 1, pack.calls)
	assert.Zero(t, hris.calls)

	// target_type lain di source yang sama jatuh ke runner per source.
	n, err = r.Load(context.Background(), "HR_OT_HOURS", "V", hrisID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = r.Load(context.Background(), "PP_NIL", "V", hrisID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, 1, pack.calls)
	assert.Equal(t, 2, hris.calls)
}

// Source tanpa runner sama sekali (walau ada target di source lain) gagal jelas.
func TestRouter_TargetOnOtherSourceDoesNotLeak(t *testing.T) {
	erpID := uuid.New()
	lookup := lookupStub{byID: map[uuid.UUID]string{erpID: bietl.SourceERPOracle}}
	pack := &runnerStub{}

	r := bietl.NewRouter(lookup)
	r.RegisterTarget(bietl.SourceHRISOracle, "PP_PACK", pack)

	_, err := r.Load(context.Background(), "PP_PACK", "V", erpID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
	assert.Zero(t, pack.calls)
}
