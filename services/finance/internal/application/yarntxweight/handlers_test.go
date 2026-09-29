package yarntxweight_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// fakeRepo is an in-memory yarntxweight.Repository.
type fakeRepo struct {
	rows      map[uuid.UUID]*yarntxweight.Entity
	types     map[int32]bool
	lastQuery yarntxweight.ListFilter
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[uuid.UUID]*yarntxweight.Entity{}, types: map[int32]bool{1: true, 2: true}}
}

func (f *fakeRepo) Create(_ context.Context, e *yarntxweight.Entity) error {
	f.rows[e.ID()] = e
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*yarntxweight.Entity, error) {
	e, ok := f.rows[id]
	if !ok || e.IsDeleted() {
		return nil, yarntxweight.ErrNotFound
	}
	return e, nil
}

func (f *fakeRepo) List(_ context.Context, filter yarntxweight.ListFilter) ([]*yarntxweight.Entity, int64, error) {
	f.lastQuery = filter
	var out []*yarntxweight.Entity
	for _, e := range f.rows {
		if e.IsDeleted() {
			continue
		}
		if filter.ProductTypeID > 0 && e.ProductTypeID() != filter.ProductTypeID {
			continue
		}
		out = append(out, e)
	}
	return out, int64(len(out)), nil
}

func (f *fakeRepo) Update(_ context.Context, e *yarntxweight.Entity) error {
	f.rows[e.ID()] = e
	return nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID, by string) error {
	e, ok := f.rows[id]
	if !ok || e.IsDeleted() {
		return yarntxweight.ErrNotFound
	}
	return e.SoftDelete(by)
}

func (f *fakeRepo) ExistsByTypeGrade(_ context.Context, pt int32, g yarntxweight.Grade) (bool, error) {
	for _, e := range f.rows {
		if !e.IsDeleted() && e.ProductTypeID() == pt && e.Grade() == g {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) ProductTypeExists(_ context.Context, pt int32) (bool, error) {
	return f.types[pt], nil
}

func createCmd(pt int32, g yarntxweight.Grade) app.CreateCommand {
	return app.CreateCommand{ProductTypeID: pt, Grade: g, Mode: yarntxweight.ModeLessBy, Value: 0.5, CreatedBy: "admin"}
}

func TestCreateHandler(t *testing.T) {
	repo := newFakeRepo()
	h := app.NewCreateHandler(repo)

	e, err := h.Handle(context.Background(), createCmd(1, yarntxweight.GradeAE))
	require.NoError(t, err)
	assert.Equal(t, int32(1), e.ProductTypeID())

	_, err = h.Handle(context.Background(), createCmd(1, yarntxweight.GradeAE))
	assert.ErrorIs(t, err, yarntxweight.ErrAlreadyExists)

	_, err = h.Handle(context.Background(), createCmd(99, yarntxweight.GradeAE))
	assert.ErrorIs(t, err, yarntxweight.ErrProductTypeNotFound)

	_, err = h.Handle(context.Background(), createCmd(1, "Q"))
	assert.ErrorIs(t, err, yarntxweight.ErrInvalidGrade)
}

func TestCreateAfterDeleteAllowed(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	e, err := app.NewCreateHandler(repo).Handle(ctx, createCmd(2, yarntxweight.GradeB))
	require.NoError(t, err)
	require.NoError(t, app.NewDeleteHandler(repo).Handle(ctx, app.DeleteCommand{ID: e.ID(), DeletedBy: "admin"}))
	_, err = app.NewCreateHandler(repo).Handle(ctx, createCmd(2, yarntxweight.GradeB))
	assert.NoError(t, err)
}

func TestGetUpdateDelete(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	e, err := app.NewCreateHandler(repo).Handle(ctx, createCmd(1, yarntxweight.GradeA))
	require.NoError(t, err)

	got, err := app.NewGetHandler(repo).Handle(ctx, app.GetQuery{ID: e.ID()})
	require.NoError(t, err)
	assert.Equal(t, e.ID(), got.ID())

	mode := yarntxweight.ModeMultiply
	val := 0.35
	upd, err := app.NewUpdateHandler(repo).Handle(ctx, app.UpdateCommand{ID: e.ID(), Mode: &mode, Value: &val, UpdatedBy: "editor"})
	require.NoError(t, err)
	assert.Equal(t, yarntxweight.ModeMultiply, upd.Mode())
	assert.InDelta(t, 0.35, upd.Value(), 1e-12)

	require.NoError(t, app.NewDeleteHandler(repo).Handle(ctx, app.DeleteCommand{ID: e.ID(), DeletedBy: "admin"}))
	_, err = app.NewGetHandler(repo).Handle(ctx, app.GetQuery{ID: e.ID()})
	assert.ErrorIs(t, err, yarntxweight.ErrNotFound)
	_, err = app.NewUpdateHandler(repo).Handle(ctx, app.UpdateCommand{ID: e.ID(), Value: &val, UpdatedBy: "editor"})
	assert.ErrorIs(t, err, yarntxweight.ErrNotFound)
}

func TestListHandler(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	for _, g := range yarntxweight.AllGrades {
		_, err := app.NewCreateHandler(repo).Handle(ctx, createCmd(1, g))
		require.NoError(t, err)
	}
	_, err := app.NewCreateHandler(repo).Handle(ctx, createCmd(2, yarntxweight.GradeC))
	require.NoError(t, err)

	res, err := app.NewListHandler(repo).Handle(ctx, app.ListQuery{Page: 1, PageSize: 2, ProductTypeID: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(5), res.TotalItems)
	assert.Equal(t, int32(3), res.TotalPages)
	assert.Equal(t, "product_type", repo.lastQuery.SortBy, "default sort applied")
}
