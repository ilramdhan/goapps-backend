package customer_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	customerapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/customer"
	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
)

// fakeRepo is an in-memory customer repository keyed on the normalized code.
type fakeRepo struct {
	byCode   map[string]*customerdomain.Customer
	nextID   int64
	upserts  []customerdomain.Sourced
	outcomes map[string]customerdomain.UpsertOutcome
	err      error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byCode:   map[string]*customerdomain.Customer{},
		nextID:   1,
		outcomes: map[string]customerdomain.UpsertOutcome{},
	}
}

func (r *fakeRepo) Create(_ context.Context, entity *customerdomain.Customer) error {
	if r.err != nil {
		return r.err
	}
	if _, ok := r.byCode[entity.Code()]; ok {
		return customerdomain.ErrAlreadyExists
	}
	entity.SetID(r.nextID)
	r.nextID++
	r.byCode[entity.Code()] = entity
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*customerdomain.Customer, error) {
	for _, c := range r.byCode {
		if c.ID() == id {
			return c, nil
		}
	}
	return nil, customerdomain.ErrNotFound
}

func (r *fakeRepo) GetByCode(_ context.Context, code string) (*customerdomain.Customer, error) {
	if r.err != nil {
		return nil, r.err
	}
	if c, ok := r.byCode[customerdomain.NormalizeCode(code)]; ok {
		return c, nil
	}
	return nil, customerdomain.ErrNotFound
}

func (r *fakeRepo) List(
	_ context.Context,
	_ customerdomain.ListFilter,
) ([]*customerdomain.Customer, int64, error) {
	items := r.all()
	return items, int64(len(items)), nil
}

func (r *fakeRepo) ListAll(_ context.Context, _ customerdomain.ListFilter) ([]*customerdomain.Customer, error) {
	return r.all(), r.err
}

func (r *fakeRepo) all() []*customerdomain.Customer {
	items := make([]*customerdomain.Customer, 0, len(r.byCode))
	for _, c := range r.byCode {
		items = append(items, c)
	}
	return items
}

func (r *fakeRepo) Update(_ context.Context, entity *customerdomain.Customer) error {
	if r.err != nil {
		return r.err
	}
	r.byCode[entity.Code()] = entity
	return nil
}

func (r *fakeRepo) UpsertSourced(
	_ context.Context,
	src customerdomain.Sourced,
) (customerdomain.UpsertOutcome, error) {
	if r.err != nil {
		return customerdomain.OutcomeSkipped, r.err
	}
	r.upserts = append(r.upserts, src)
	if o, ok := r.outcomes[src.Code]; ok {
		return o, nil
	}
	return customerdomain.OutcomeInserted, nil
}

func (r *fakeRepo) ResolveCodes(_ context.Context, codes []string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, code := range codes {
		if c, ok := r.byCode[customerdomain.NormalizeCode(code)]; ok {
			out[customerdomain.NormalizeCode(code)] = c.ID()
		}
	}
	return out, r.err
}

// fakeSource replays scripted Oracle rows.
type fakeSource struct {
	rows []customerdomain.Sourced
	err  error
}

func (s *fakeSource) ListCustomers(_ context.Context) ([]customerdomain.Sourced, error) {
	return s.rows, s.err
}

func seed(t *testing.T, repo *fakeRepo, code, name string) *customerdomain.Customer {
	t.Helper()
	entity, err := customerdomain.New(customerdomain.NewParams{Code: code, Name: name, CreatedBy: "seed"})
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), entity))
	return entity
}

func TestCreate_PersistsAndAssignsID(t *testing.T) {
	repo := newFakeRepo()
	svc := customerapp.NewService(repo)

	entity, err := svc.Create(context.Background(), customerapp.CreateCommand{
		Code: "dc00594", Name: "PT. BESTOW", CreatedBy: "planner",
	})
	require.NoError(t, err)
	assert.Equal(t, "DC00594", entity.Code())
	assert.Equal(t, int64(1), entity.ID())
	assert.Equal(t, customerdomain.SourceManual, entity.Source())
}

func TestCreate_InvalidCommand_DoesNotTouchRepository(t *testing.T) {
	repo := newFakeRepo()
	svc := customerapp.NewService(repo)

	_, err := svc.Create(context.Background(), customerapp.CreateCommand{Code: "", Name: "X", CreatedBy: "p"})
	require.ErrorIs(t, err, customerdomain.ErrEmptyCode)
	assert.Empty(t, repo.byCode)
}

func TestUpdate_AppliesChanges(t *testing.T) {
	repo := newFakeRepo()
	existing := seed(t, repo, "DC1", "OLD")
	svc := customerapp.NewService(repo)

	newName := "NEW"
	entity, err := svc.Update(context.Background(), customerapp.UpdateCommand{
		ID: existing.ID(), Name: &newName, UpdatedBy: "editor",
	})
	require.NoError(t, err)
	assert.Equal(t, "NEW", entity.Name())
}

func TestUpdate_UnknownID_ReturnsNotFound(t *testing.T) {
	svc := customerapp.NewService(newFakeRepo())
	_, err := svc.Update(context.Background(), customerapp.UpdateCommand{ID: 99, UpdatedBy: "e"})
	require.ErrorIs(t, err, customerdomain.ErrNotFound)
}

func TestList_ComputesPagination(t *testing.T) {
	repo := newFakeRepo()
	for _, code := range []string{"A1", "A2", "A3"} {
		seed(t, repo, code, "N-"+code)
	}
	svc := customerapp.NewService(repo)

	res, err := svc.List(context.Background(), customerapp.ListQuery{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.TotalItems)
	assert.Equal(t, int32(2), res.TotalPages)
	assert.Equal(t, int32(1), res.CurrentPage)
	assert.Equal(t, int32(2), res.PageSize)
}

func TestSyncFromOracle_NotConfigured(t *testing.T) {
	svc := customerapp.NewService(newFakeRepo())
	_, err := svc.SyncFromOracle(context.Background())
	require.ErrorIs(t, err, customerdomain.ErrSyncNotConfigured)
}

func TestSync_CountsOutcomes(t *testing.T) {
	repo := newFakeRepo()
	repo.outcomes = map[string]customerdomain.UpsertOutcome{
		"A": customerdomain.OutcomeInserted,
		"B": customerdomain.OutcomeUpdated,
		"C": customerdomain.OutcomeSkipped,
	}
	src := &fakeSource{rows: []customerdomain.Sourced{
		{Code: "A", Name: "Alpha", IsActive: true},
		{Code: "B", Name: "Bravo", IsActive: true},
		{Code: "C", Name: "Charlie", IsActive: false},
	}}
	svc := customerapp.NewService(repo).WithSync(customerapp.NewSyncUsecase(repo, src))

	res, err := svc.SyncFromOracle(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 1, res.Unchanged)
	assert.True(t, res.SourceUsed)
	assert.Len(t, repo.upserts, 3)
}

func TestSync_NilSource_IsNoOp(t *testing.T) {
	repo := newFakeRepo()
	usecase := customerapp.NewSyncUsecase(repo, nil)

	res, err := usecase.Sync(context.Background())
	require.NoError(t, err)
	assert.False(t, res.SourceUsed)
	assert.Empty(t, repo.upserts)
}

func TestSync_SourceError_IsReported(t *testing.T) {
	repo := newFakeRepo()
	boom := errors.New("oracle down")
	usecase := customerapp.NewSyncUsecase(repo, &fakeSource{err: boom})

	_, err := usecase.Sync(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestSync_IsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	repo.outcomes = map[string]customerdomain.UpsertOutcome{"A": customerdomain.OutcomeSkipped}
	usecase := customerapp.NewSyncUsecase(repo, &fakeSource{
		rows: []customerdomain.Sourced{{Code: "A", Name: "Alpha", IsActive: true}},
	})

	first, err := usecase.Sync(context.Background())
	require.NoError(t, err)
	second, err := usecase.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, first, second, "re-running with unchanged source data must report the same counts")
	assert.Equal(t, 1, second.Unchanged)
}

func TestResolveCodes(t *testing.T) {
	repo := newFakeRepo()
	known := seed(t, repo, "DC00594", "PT. BESTOW")
	svc := customerapp.NewService(repo)

	got, err := svc.ResolveCodes(context.Background(), []string{" dc00594 ", "MISSING"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{"DC00594": known.ID()}, got)
}
