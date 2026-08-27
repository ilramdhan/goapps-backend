package grpc

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// fakeShadeRepo is an in-memory test double for shade.Repository. It is enough
// to exercise the delivery-layer mapping and, most importantly, to prove that
// a manually created/updated row is stored with ces_shade_source = MANUAL and
// that UpsertSourced (the sync path) never overwrites it.
type fakeShadeRepo struct {
	byID       map[int64]*shade.Shade
	byCode     map[string]*shade.Shade
	nextID     int64
	upsertCall int
}

func newFakeShadeRepo() *fakeShadeRepo {
	return &fakeShadeRepo{byID: map[int64]*shade.Shade{}, byCode: map[string]*shade.Shade{}}
}

func (f *fakeShadeRepo) Create(_ context.Context, entity *shade.Shade) error {
	f.nextID++
	entity.SetID(f.nextID)
	f.byID[entity.ID()] = entity
	f.byCode[entity.Code()] = entity
	return nil
}

func (f *fakeShadeRepo) GetByID(_ context.Context, id int64) (*shade.Shade, error) {
	e, ok := f.byID[id]
	if !ok {
		return nil, shade.ErrNotFound
	}
	return e, nil
}

func (f *fakeShadeRepo) GetByCode(_ context.Context, code string) (*shade.Shade, error) {
	e, ok := f.byCode[code]
	if !ok {
		return nil, shade.ErrNotFound
	}
	return e, nil
}

func (f *fakeShadeRepo) List(_ context.Context, _ shade.ListFilter) ([]*shade.Shade, int64, error) {
	items := make([]*shade.Shade, 0, len(f.byID))
	for _, e := range f.byID {
		items = append(items, e)
	}
	return items, int64(len(items)), nil
}

func (f *fakeShadeRepo) Update(_ context.Context, entity *shade.Shade) error {
	f.byID[entity.ID()] = entity
	f.byCode[entity.Code()] = entity
	return nil
}

// UpsertSourced mirrors the real postgres.ShadeRepository's MANUAL guard: a row
// whose existing provenance is MANUAL is left untouched and reported skipped.
// This is the same decideUpsertAction contract the production repository
// implements; re-asserting it here at the fake level lets the handler test
// below prove end-to-end that Create -> Sync cannot clobber a manual row.
func (f *fakeShadeRepo) UpsertSourced(_ context.Context, src shade.Sourced) (shade.UpsertOutcome, error) {
	f.upsertCall++
	existing, ok := f.byCode[src.Code]
	if ok && existing.Source() == shade.SourceManual {
		return shade.OutcomeSkipped, nil
	}
	if ok {
		_ = existing.Update(shade.UpdateParams{Name: &src.Name, UpdatedBy: "sync"})
		return shade.OutcomeUpdated, nil
	}
	f.nextID++
	entity := shade.Reconstruct(shade.ReconstructParams{
		ID: f.nextID, Code: src.Code, Name: src.Name, IsActive: src.IsActive,
		Source: shade.SourceOracle, CreatedBy: "sync",
	})
	f.byID[entity.ID()] = entity
	f.byCode[entity.Code()] = entity
	return shade.OutcomeInserted, nil
}

var _ shade.Repository = (*fakeShadeRepo)(nil)

// fakeShadeSource is a test double for shade.Source (the Oracle reader).
type fakeShadeSource struct {
	items []shade.Sourced
	err   error
}

func (f *fakeShadeSource) ListShades(_ context.Context) ([]shade.Sourced, error) {
	return f.items, f.err
}

func newShadeHandlerForTest(t *testing.T, oracle shade.Source) (*ShadeHandler, *fakeShadeRepo) {
	t.Helper()
	repo := newFakeShadeRepo()
	h, err := NewShadeHandler(
		appshade.NewCreateHandler(repo),
		appshade.NewGetHandler(repo),
		appshade.NewListHandler(repo),
		appshade.NewUpdateHandler(repo),
		appshade.NewSyncHandler(oracle, repo, zerolog.New(io.Discard)),
	)
	require.NoError(t, err)
	return h, repo
}

func TestCreateShade_StoresRowAsManual(t *testing.T) {
	h, repo := newShadeHandlerForTest(t, nil)

	resp, err := h.CreateShade(context.Background(), &financev1.CreateShadeRequest{
		ShadeCode: "Z999S",
		ShadeName: "Test Shade",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	require.True(t, resp.GetBase().GetIsSuccess(), "expected success, got: %s", resp.GetBase().GetMessage())
	assert.Equal(t, shade.SourceManual, resp.GetData().GetShadeSource())

	// Directly confirm the persisted row (not just the response DTO) carries
	// ces_shade_source = MANUAL.
	stored, getErr := repo.GetByCode(context.Background(), "Z999S")
	require.NoError(t, getErr)
	assert.Equal(t, shade.SourceManual, stored.Source())
}

func TestSyncShades_NeverOverwritesManualRow(t *testing.T) {
	h, repo := newShadeHandlerForTest(t, &fakeShadeSource{
		items: []shade.Sourced{
			{Code: "Z999S", Name: "Oracle-side rename attempt", IsActive: true},
		},
	})

	createResp, err := h.CreateShade(context.Background(), &financev1.CreateShadeRequest{
		ShadeCode: "Z999S",
		ShadeName: "Manual Name",
	})
	require.NoError(t, err)
	require.True(t, createResp.GetBase().GetIsSuccess())

	syncResp, err := h.SyncShades(context.Background(), &financev1.SyncShadesRequest{})
	require.NoError(t, err)
	require.True(t, syncResp.GetBase().GetIsSuccess(), "expected success, got: %s", syncResp.GetBase().GetMessage())
	assert.Equal(t, int32(1), syncResp.GetSkipped())
	assert.Equal(t, int32(0), syncResp.GetUpdated())

	stored, getErr := repo.GetByCode(context.Background(), "Z999S")
	require.NoError(t, getErr)
	assert.Equal(t, "Manual Name", stored.Name(), "sync must not overwrite a MANUAL row's name")
	assert.Equal(t, shade.SourceManual, stored.Source())
}

func TestSyncShades_NotConfiguredReportsErrorNotCrash(t *testing.T) {
	h, _ := newShadeHandlerForTest(t, nil)

	resp, err := h.SyncShades(context.Background(), &financev1.SyncShadesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.False(t, resp.GetBase().GetIsSuccess())
	assert.Equal(t, "409", resp.GetBase().GetStatusCode())
}

func TestDeactivateShade_SoftDeactivatesNotDelete(t *testing.T) {
	h, repo := newShadeHandlerForTest(t, nil)

	createResp, err := h.CreateShade(context.Background(), &financev1.CreateShadeRequest{
		ShadeCode: "Z998S",
		ShadeName: "To Deactivate",
	})
	require.NoError(t, err)
	id := createResp.GetData().GetShadeId()

	resp, err := h.DeactivateShade(context.Background(), &financev1.DeactivateShadeRequest{ShadeId: id})
	require.NoError(t, err)
	require.True(t, resp.GetBase().GetIsSuccess())
	assert.False(t, resp.GetData().GetIsActive())

	// The row still exists (soft, not hard, delete) and is retrievable.
	stored, getErr := repo.GetByID(context.Background(), id)
	require.NoError(t, getErr)
	assert.False(t, stored.IsActive())
}

func TestGetShade_NotFoundMapsToNotFoundResponse(t *testing.T) {
	h, _ := newShadeHandlerForTest(t, nil)

	resp, err := h.GetShade(context.Background(), &financev1.GetShadeRequest{ShadeId: 999})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.False(t, resp.GetBase().GetIsSuccess())
}

func TestListShades_ActiveFilterMapsToQuery(t *testing.T) {
	h, _ := newShadeHandlerForTest(t, nil)

	resp, err := h.ListShades(context.Background(), &financev1.ListShadesRequest{
		Page:         1,
		PageSize:     10,
		ActiveFilter: financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.True(t, resp.GetBase().GetIsSuccess())
	require.NotNil(t, resp.GetPagination())
	assert.Equal(t, int32(1), resp.GetPagination().GetCurrentPage())
}
