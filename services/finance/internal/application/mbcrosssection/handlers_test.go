package mbcrosssection_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcrosssection"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// =============================================================================
// fakeRepo — in-memory test double for domain.Repository. Captures the entity
// handed to Create/Update and the filter handed to List.
// =============================================================================
type fakeRepo struct {
	created   *domain.Entity
	updated   *domain.Entity
	gotFilter domain.ListFilter
	deletedID string
	deletedBy string

	getEntity *domain.Entity
	getErr    error
	listItems []*domain.Entity
	listTotal int64
	listErr   error
	createErr error
	updateErr error
	deleteErr error
}

func (f *fakeRepo) Create(_ context.Context, e *domain.Entity) error {
	f.created = e
	return f.createErr
}

func (f *fakeRepo) Update(_ context.Context, e *domain.Entity) error {
	f.updated = e
	return f.updateErr
}

func (f *fakeRepo) Delete(_ context.Context, id, deletedBy string) error {
	f.deletedID, f.deletedBy = id, deletedBy
	return f.deleteErr
}

func (f *fakeRepo) GetByID(_ context.Context, _ string) (*domain.Entity, error) {
	return f.getEntity, f.getErr
}

func (f *fakeRepo) GetByCode(_ context.Context, _ string) (*domain.Entity, error) {
	return f.getEntity, f.getErr
}

func (f *fakeRepo) List(_ context.Context, filter domain.ListFilter) ([]*domain.Entity, int64, error) {
	f.gotFilter = filter
	return f.listItems, f.listTotal, f.listErr
}

var _ domain.Repository = (*fakeRepo)(nil)

// =============================================================================
// CreateHandler
// =============================================================================

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - persists a new active cross section", func(t *testing.T) {
		repo := &fakeRepo{}
		got, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{
			Code:         "RND",
			DisplayName:  "Round",
			Description:  "circular",
			DisplayOrder: 1,
			IsActive:     true,
			CreatedBy:    "admin",
		})

		require.NoError(t, err)
		assert.Equal(t, "RND", got.Code())
		assert.Equal(t, "Round", got.DisplayName())
		assert.True(t, got.IsActive())
		assert.Same(t, got, repo.created)
	})

	t.Run("honours an explicit inactive create", func(t *testing.T) {
		repo := &fakeRepo{}
		got, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{
			Code: "RSD", CreatedBy: "admin", IsActive: false,
		})

		require.NoError(t, err)
		assert.False(t, got.IsActive())
	})

	t.Run("rejects an empty code without touching the repo", func(t *testing.T) {
		repo := &fakeRepo{}
		_, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{CreatedBy: "admin"})

		require.ErrorIs(t, err, domain.ErrCodeRequired)
		assert.Nil(t, repo.created)
	})

	t.Run("propagates a duplicate-code repo error", func(t *testing.T) {
		repo := &fakeRepo{createErr: domain.ErrAlreadyExists}
		_, err := app.NewCreateHandler(repo).Handle(context.Background(), app.CreateCommand{
			Code: "RND", CreatedBy: "admin", IsActive: true,
		})

		require.ErrorIs(t, err, domain.ErrAlreadyExists)
	})
}

// =============================================================================
// GetHandler / DeleteHandler
// =============================================================================

func TestGetHandler_Handle(t *testing.T) {
	existing := domain.Reconstruct("id-1", "RND", "Round", "", 1, true, "t0", "admin", "", "", "", "")
	repo := &fakeRepo{getEntity: existing}

	got, err := app.NewGetHandler(repo).Handle(context.Background(), app.GetQuery{ID: "id-1"})

	require.NoError(t, err)
	assert.Same(t, existing, got)
}

func TestGetHandler_Handle_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: domain.ErrNotFound}

	_, err := app.NewGetHandler(repo).Handle(context.Background(), app.GetQuery{ID: "missing"})

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteHandler_Handle(t *testing.T) {
	repo := &fakeRepo{}

	err := app.NewDeleteHandler(repo).Handle(context.Background(), app.DeleteCommand{ID: "id-1", DeletedBy: "admin"})

	require.NoError(t, err)
	assert.Equal(t, "id-1", repo.deletedID)
	assert.Equal(t, "admin", repo.deletedBy)
}

// =============================================================================
// UpdateHandler
// =============================================================================

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - code stays immutable, editable fields change", func(t *testing.T) {
		existing := domain.Reconstruct("id-1", "RND", "Round", "old", 1, true, "t0", "admin", "", "", "", "")
		repo := &fakeRepo{getEntity: existing}

		got, err := app.NewUpdateHandler(repo).Handle(context.Background(), app.UpdateCommand{
			ID:           "id-1",
			DisplayName:  "Round v2",
			Description:  "new",
			DisplayOrder: 9,
			IsActive:     false,
			UpdatedBy:    "editor",
		})

		require.NoError(t, err)
		assert.Equal(t, "RND", got.Code(), "code must never change on update")
		assert.Equal(t, "Round v2", got.DisplayName())
		assert.Equal(t, "new", got.Description())
		assert.Equal(t, int32(9), got.DisplayOrder())
		assert.False(t, got.IsActive())
		assert.Equal(t, "editor", got.UpdatedBy())
		assert.Same(t, got, repo.updated)
	})

	t.Run("rejects an update on a soft-deleted row", func(t *testing.T) {
		deleted := domain.Reconstruct("id-1", "RND", "Round", "", 1, true, "t0", "admin", "", "", "t9", "admin")
		repo := &fakeRepo{getEntity: deleted}

		_, err := app.NewUpdateHandler(repo).Handle(context.Background(), app.UpdateCommand{
			ID: "id-1", UpdatedBy: "editor",
		})

		require.ErrorIs(t, err, domain.ErrDeleted)
		assert.Nil(t, repo.updated)
	})

	t.Run("rejects a missing updated_by", func(t *testing.T) {
		existing := domain.Reconstruct("id-1", "RND", "Round", "", 1, true, "t0", "admin", "", "", "", "")
		repo := &fakeRepo{getEntity: existing}

		_, err := app.NewUpdateHandler(repo).Handle(context.Background(), app.UpdateCommand{ID: "id-1"})

		require.ErrorIs(t, err, domain.ErrUpdatedByRequired)
	})

	t.Run("propagates a not-found lookup", func(t *testing.T) {
		repo := &fakeRepo{getErr: domain.ErrNotFound}

		_, err := app.NewUpdateHandler(repo).Handle(context.Background(), app.UpdateCommand{ID: "missing", UpdatedBy: "editor"})

		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

// =============================================================================
// ListHandler
// =============================================================================

func TestListHandler_Handle(t *testing.T) {
	t.Run("maps the query onto the filter and computes total pages", func(t *testing.T) {
		active := true
		repo := &fakeRepo{listTotal: 21}

		got, err := app.NewListHandler(repo).Handle(context.Background(), app.ListQuery{
			Page: 2, PageSize: 10, Search: "rn", SortBy: "code", SortOrder: "desc", IsActive: &active,
		})

		require.NoError(t, err)
		assert.Equal(t, "rn", repo.gotFilter.Search)
		assert.Equal(t, "code", repo.gotFilter.SortBy)
		assert.Equal(t, "desc", repo.gotFilter.SortOrder)
		require.NotNil(t, repo.gotFilter.IsActive)
		assert.True(t, *repo.gotFilter.IsActive)
		assert.Equal(t, int64(21), got.TotalItems)
		assert.Equal(t, int32(3), got.TotalPages, "21 rows over a page size of 10 is 3 pages")
		assert.Equal(t, int32(2), got.CurrentPage)
	})

	t.Run("applies filter defaults for a zero-valued query", func(t *testing.T) {
		repo := &fakeRepo{}

		got, err := app.NewListHandler(repo).Handle(context.Background(), app.ListQuery{})

		require.NoError(t, err)
		assert.Equal(t, int32(1), repo.gotFilter.Page)
		assert.Equal(t, int32(10), repo.gotFilter.PageSize)
		assert.Equal(t, "display_order", repo.gotFilter.SortBy)
		assert.Equal(t, "asc", repo.gotFilter.SortOrder)
		assert.Equal(t, int32(0), got.TotalPages, "no rows means no pages")
	})

	t.Run("propagates a repo error", func(t *testing.T) {
		boom := errors.New("boom")
		repo := &fakeRepo{listErr: boom}

		_, err := app.NewListHandler(repo).Handle(context.Background(), app.ListQuery{})

		require.ErrorIs(t, err, boom)
	})
}
