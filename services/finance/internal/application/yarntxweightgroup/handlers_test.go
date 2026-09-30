package yarntxweightgroup_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/yarntxweightgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// fakeRepo is an in-memory yarntxweightgroup.Repository. mapping models the
// junction table's UNIQUE(product_type_id).
type fakeRepo struct {
	rows      map[uuid.UUID]*yarntxweightgroup.Entity
	types     map[int32]string // id -> code
	mapping   map[int32]uuid.UUID
	lastQuery yarntxweightgroup.ListFilter
	creates   int
	updates   int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rows:    map[uuid.UUID]*yarntxweightgroup.Entity{},
		types:   map[int32]string{1: "TTY", 2: "TTS", 3: "DTY", 4: "PTY", 5: "POY"},
		mapping: map[int32]uuid.UUID{},
	}
}

func (f *fakeRepo) store(e *yarntxweightgroup.Entity) {
	for pt, gid := range f.mapping {
		if gid == e.ID() {
			delete(f.mapping, pt)
		}
	}
	for _, id := range e.ProductTypeIDs() {
		f.mapping[id] = e.ID()
	}
	f.rows[e.ID()] = e
}

func (f *fakeRepo) Create(_ context.Context, e *yarntxweightgroup.Entity) error {
	f.creates++
	f.store(e)
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*yarntxweightgroup.Entity, error) {
	e, ok := f.rows[id]
	if !ok || e.IsDeleted() {
		return nil, yarntxweightgroup.ErrNotFound
	}
	return e, nil
}

func (f *fakeRepo) List(_ context.Context, filter yarntxweightgroup.ListFilter) ([]*yarntxweightgroup.Entity, int64, error) {
	f.lastQuery = filter
	var out []*yarntxweightgroup.Entity
	for _, e := range f.rows {
		if e.IsDeleted() {
			continue
		}
		if filter.ProductTypeID > 0 && f.mapping[filter.ProductTypeID] != e.ID() {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code() < out[j].Code() })
	return out, int64(len(out)), nil
}

func (f *fakeRepo) Update(_ context.Context, e *yarntxweightgroup.Entity) error {
	f.updates++
	f.store(e)
	return nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID, by string) error {
	e, ok := f.rows[id]
	if !ok || e.IsDeleted() {
		return yarntxweightgroup.ErrNotFound
	}
	for pt, gid := range f.mapping {
		if gid == id {
			delete(f.mapping, pt)
		}
	}
	return e.SoftDelete(by)
}

func (f *fakeRepo) ExistsByCode(_ context.Context, code string, excludeID uuid.UUID) (bool, error) {
	for _, e := range f.rows {
		if !e.IsDeleted() && e.Code() == code && e.ID() != excludeID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) MissingProductTypes(_ context.Context, ids []int32) ([]int32, error) {
	var missing []int32
	for _, id := range ids {
		if _, ok := f.types[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

func (f *fakeRepo) FindProductTypeConflicts(_ context.Context, ids []int32, excludeID uuid.UUID) ([]yarntxweightgroup.ProductTypeConflict, error) {
	var out []yarntxweightgroup.ProductTypeConflict
	for _, id := range ids {
		gid, ok := f.mapping[id]
		if !ok || gid == excludeID {
			continue
		}
		out = append(out, yarntxweightgroup.ProductTypeConflict{
			ProductTypeID: id, ProductTypeCode: f.types[id], GroupCode: f.rows[gid].Code(),
		})
	}
	return out, nil
}

func rules(grades ...yarntxweight.Grade) []yarntxweightgroup.Rule {
	out := make([]yarntxweightgroup.Rule, len(grades))
	for i, g := range grades {
		out[i] = yarntxweightgroup.Rule{Grade: g, Mode: yarntxweight.ModeFixed, Value: float64(i + 1)}
	}
	return out
}

func input(code string, types ...int32) yarntxweightgroup.Input {
	return yarntxweightgroup.Input{
		Code: code, Name: code + " config", ProductTypeIDs: types,
		Rules: rules(yarntxweight.GradeAE, yarntxweight.GradeB),
	}
}

func mustCreate(t *testing.T, repo *fakeRepo, in yarntxweightgroup.Input) *yarntxweightgroup.Entity {
	t.Helper()
	e, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{Input: in, CreatedBy: "admin"})
	require.NoError(t, err)
	return e
}

func TestCreateHandler_Success(t *testing.T) {
	repo := newFakeRepo()
	e := mustCreate(t, repo, input("TTY", 1, 2))
	assert.Equal(t, "TTY", e.Code())
	assert.Equal(t, []int32{1, 2}, e.ProductTypeIDs())
	assert.Equal(t, e.ID(), repo.mapping[1])
	assert.Equal(t, e.ID(), repo.mapping[2])
}

func TestCreateHandler_TypeAlreadyMappedToAnotherGroup(t *testing.T) {
	repo := newFakeRepo()
	mustCreate(t, repo, input("DTY", 3, 4))

	_, err := app.NewCreateHandler(repo).Handle(context.Background(),
		app.CreateCommand{Input: input("PTY_OWN", 4, 5), CreatedBy: "admin"})
	require.ErrorIs(t, err, yarntxweightgroup.ErrProductTypeAlreadyMapped)
	var conflict *yarntxweightgroup.ProductTypeConflictError
	require.True(t, errors.As(err, &conflict))
	require.Len(t, conflict.Conflicts, 1)
	assert.Equal(t, "PTY", conflict.Conflicts[0].ProductTypeCode)
	assert.Equal(t, "DTY", conflict.Conflicts[0].GroupCode)
	assert.Contains(t, err.Error(), "product type PTY is already assigned to tx weight group DTY")
	assert.Equal(t, 1, repo.creates, "nothing persisted on conflict")
	_, mapped := repo.mapping[5]
	assert.False(t, mapped, "free type must not be half-mapped")
}

func TestCreateHandler_DuplicateGrade(t *testing.T) {
	repo := newFakeRepo()
	in := input("TTY", 1)
	in.Rules = rules(yarntxweight.GradeA, yarntxweight.GradeA)
	_, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{Input: in, CreatedBy: "admin"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrDuplicateGrade)
	assert.Zero(t, repo.creates)
}

func TestCreateHandler_Rejections(t *testing.T) {
	repo := newFakeRepo()
	mustCreate(t, repo, input("TTY", 1))
	ctx := context.Background()
	h := app.NewCreateHandler(repo)

	_, err := h.Handle(ctx, app.CreateCommand{Input: input("tty", 2), CreatedBy: "admin"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrCodeAlreadyExists, "code compared after normalization")

	_, err = h.Handle(ctx, app.CreateCommand{Input: input("NEW", 2, 99), CreatedBy: "admin"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrProductTypeNotFound)

	_, err = h.Handle(ctx, app.CreateCommand{Input: input("NEW", 2, 2), CreatedBy: "admin"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrDuplicateProductType)
	assert.Equal(t, 1, repo.creates)
}

func TestUpdateHandler_ReplacesSet(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	e := mustCreate(t, repo, input("TTY", 1, 2))

	in := input("TTY", 2, 5)
	in.Rules = rules(yarntxweight.GradeC)
	upd, err := app.NewUpdateHandler(repo).Handle(ctx, app.UpdateCommand{ID: e.ID(), Input: in, UpdatedBy: "editor"})
	require.NoError(t, err)
	assert.Equal(t, []int32{2, 5}, upd.ProductTypeIDs())
	require.Len(t, upd.Rules(), 1)
	assert.Equal(t, yarntxweight.GradeC, upd.Rules()[0].Grade)
	_, stillMapped := repo.mapping[1]
	assert.False(t, stillMapped, "type dropped from the set becomes free")
	assert.Equal(t, e.ID(), repo.mapping[5])

	// Type 1 is free again, so another group can take it.
	mustCreate(t, repo, input("OTHER", 1))
}

func TestUpdateHandler_KeepsOwnTypesAndCode(t *testing.T) {
	repo := newFakeRepo()
	e := mustCreate(t, repo, input("TTY", 1, 2))
	_, err := app.NewUpdateHandler(repo).Handle(context.Background(),
		app.UpdateCommand{ID: e.ID(), Input: input("TTY", 1, 2), UpdatedBy: "editor"})
	require.NoError(t, err, "own code and own types are not conflicts")
}

func TestUpdateHandler_TypeAlreadyMappedToAnotherGroup(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	dty := mustCreate(t, repo, input("DTY", 3))
	tty := mustCreate(t, repo, input("TTY", 1))

	_, err := app.NewUpdateHandler(repo).Handle(ctx,
		app.UpdateCommand{ID: tty.ID(), Input: input("TTY", 1, 3), UpdatedBy: "editor"})
	require.ErrorIs(t, err, yarntxweightgroup.ErrProductTypeAlreadyMapped)
	assert.Contains(t, err.Error(), "DTY is already assigned to tx weight group DTY")
	assert.Equal(t, dty.ID(), repo.mapping[3])
	assert.Zero(t, repo.updates)

	_, err = app.NewUpdateHandler(repo).Handle(ctx,
		app.UpdateCommand{ID: tty.ID(), Input: input("DTY", 1), UpdatedBy: "editor"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrCodeAlreadyExists)
}

func TestUpdateHandler_DuplicateGradeAndNotFound(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	e := mustCreate(t, repo, input("TTY", 1))
	in := input("TTY", 1)
	in.Rules = rules(yarntxweight.GradeB, yarntxweight.GradeB)
	_, err := app.NewUpdateHandler(repo).Handle(ctx, app.UpdateCommand{ID: e.ID(), Input: in, UpdatedBy: "editor"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrDuplicateGrade)

	_, err = app.NewUpdateHandler(repo).Handle(ctx, app.UpdateCommand{ID: uuid.New(), Input: input("X", 2), UpdatedBy: "editor"})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrNotFound)
}

func TestGetAndDeleteFreesTypes(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	e := mustCreate(t, repo, input("TTY", 1, 2))

	got, err := app.NewGetHandler(repo).Handle(ctx, app.GetQuery{ID: e.ID()})
	require.NoError(t, err)
	assert.Equal(t, e.ID(), got.ID())

	require.NoError(t, app.NewDeleteHandler(repo).Handle(ctx, app.DeleteCommand{ID: e.ID(), DeletedBy: "admin"}))
	_, err = app.NewGetHandler(repo).Handle(ctx, app.GetQuery{ID: e.ID()})
	assert.ErrorIs(t, err, yarntxweightgroup.ErrNotFound)
	assert.ErrorIs(t, app.NewDeleteHandler(repo).Handle(ctx, app.DeleteCommand{ID: e.ID(), DeletedBy: "admin"}),
		yarntxweightgroup.ErrNotFound)

	// Freed types and the code are reusable after delete.
	mustCreate(t, repo, input("TTY", 1, 2))
}

func TestListHandler(t *testing.T) {
	repo := newFakeRepo()
	ctx := context.Background()
	mustCreate(t, repo, input("A1", 1))
	mustCreate(t, repo, input("A2", 2))
	mustCreate(t, repo, input("A3", 3))

	res, err := app.NewListHandler(repo).Handle(ctx, app.ListQuery{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.TotalItems)
	assert.Equal(t, int32(2), res.TotalPages)
	assert.Equal(t, yarntxweightgroup.SortByCode, repo.lastQuery.SortBy, "default sort applied")

	res, err = app.NewListHandler(repo).Handle(ctx, app.ListQuery{ProductTypeID: 2})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "A2", res.Items[0].Code())
	assert.Equal(t, int32(10), res.PageSize)
}
