package superbacostsp

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

type fakeRepo struct {
	byID   map[string]*domain.Entry
	nextID int
	upsert []domain.Sourced
}

func newFake() *fakeRepo { return &fakeRepo{byID: map[string]*domain.Entry{}} }

func (f *fakeRepo) Create(_ context.Context, e *domain.Entry) error {
	f.nextID++
	e.SetID(string(rune('a' + f.nextID)))
	f.byID[e.ID()] = e
	return nil
}
func (f *fakeRepo) GetByID(_ context.Context, id string) (*domain.Entry, error) {
	if e, ok := f.byID[id]; ok {
		return e, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeRepo) GetByLegacySysID(_ context.Context, l int64) (*domain.Entry, error) {
	for _, e := range f.byID {
		if e.LegacySysID() == l {
			return e, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (f *fakeRepo) List(_ context.Context, _ domain.ListFilter) ([]*domain.Entry, int64, *time.Time, error) {
	out := make([]*domain.Entry, 0, len(f.byID))
	for _, e := range f.byID {
		out = append(out, e)
	}
	return out, int64(len(out)), nil, nil
}
func (f *fakeRepo) Update(context.Context, *domain.Entry) error { return nil }
func (f *fakeRepo) SoftDelete(_ context.Context, id, _ string) error {
	if _, ok := f.byID[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}
func (f *fakeRepo) UpsertByLegacySysID(_ context.Context, s domain.Sourced) (domain.UpsertOutcome, error) {
	f.upsert = append(f.upsert, s)
	if s.LegacySysID == 2 {
		return domain.OutcomeUpdated, nil
	}
	if s.LegacySysID == 3 {
		return domain.OutcomeUnchanged, nil
	}
	return domain.OutcomeInserted, nil
}
func (f *fakeRepo) ResolveByShades(context.Context, []string) (map[string]domain.Resolved, error) {
	return nil, nil
}

type fakeSource struct{ rows []domain.Sourced }

func (s fakeSource) ListSuperbaCostSP(context.Context) ([]domain.Sourced, error) { return s.rows, nil }

func TestCreateGetUpdateDelete(t *testing.T) {
	ctx := context.Background()
	repo := newFake()
	create := NewCreateHandler(repo)

	e, err := create.Handle(ctx, CreateCommand{LegacySysID: 10, ShadeCode: " sp1 ", OldValue: 1.5, IsActive: true, CreatedBy: "u"})
	require.NoError(t, err)
	assert.Equal(t, "SP1", e.ShadeCode())
	assert.Equal(t, domain.SourceManual, e.Source())

	_, err = create.Handle(ctx, CreateCommand{LegacySysID: 10, ShadeCode: "X", OldValue: 1, CreatedBy: "u"})
	assert.ErrorIs(t, err, domain.ErrDuplicateLegacySysID)
	_, err = create.Handle(ctx, CreateCommand{LegacySysID: 11, ShadeCode: "", OldValue: 1, CreatedBy: "u"})
	assert.ErrorIs(t, err, domain.ErrEmptyShadeCode)
	_, err = create.Handle(ctx, CreateCommand{LegacySysID: 12, ShadeCode: "A", OldValue: -1, CreatedBy: "u"})
	assert.ErrorIs(t, err, domain.ErrNegativeValue)

	got, err := NewGetHandler(repo).Handle(ctx, e.ID())
	require.NoError(t, err)
	assert.Equal(t, e, got)
	_, err = NewGetHandler(repo).Handle(ctx, "zz")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	v := 2.5
	up, err := NewUpdateHandler(repo).Handle(ctx, UpdateCommand{ID: e.ID(), OldValue: &v, UpdatedBy: "u2"})
	require.NoError(t, err)
	assert.InDelta(t, 2.5, up.OldValue(), 1e-9)
	assert.Equal(t, "SP1", up.ShadeCode())

	require.NoError(t, NewDeleteHandler(repo).Handle(ctx, e.ID(), "u"))
	assert.ErrorIs(t, NewDeleteHandler(repo).Handle(ctx, e.ID(), "u"), domain.ErrNotFound)
}

func TestListPaginationTotals(t *testing.T) {
	repo := newFake()
	for i := int64(1); i <= 3; i++ {
		_, err := NewCreateHandler(repo).Handle(context.Background(), CreateCommand{LegacySysID: i, ShadeCode: "S", OldValue: 1, CreatedBy: "u"})
		require.NoError(t, err)
	}
	res, err := NewListHandler(repo).Handle(context.Background(), ListQuery{PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.TotalItems)
	assert.Equal(t, int32(2), res.TotalPages)
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	_, err := NewSyncHandler(nil, newFake(), zerolog.Nop()).Execute(ctx)
	assert.ErrorIs(t, err, domain.ErrSyncNotConfigured)

	repo := newFake()
	src := fakeSource{rows: []domain.Sourced{{LegacySysID: 1}, {LegacySysID: 2}, {LegacySysID: 3}}}
	res, err := NewSyncHandler(src, repo, zerolog.Nop()).Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, res.TotalRows)
	assert.Equal(t, 1, res.Inserted)
	assert.Equal(t, 1, res.Updated)
	assert.Equal(t, 1, res.Unchanged)
}
