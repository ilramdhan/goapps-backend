package mbcrosssection_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcrosssection"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcrosssection"
)

// =============================================================================
// fakeFactorRepo — in-memory test double for domain.FactorRepository.
// =============================================================================
type fakeFactorRepo struct {
	created   *domain.FactorEntity
	updated   *domain.FactorEntity
	gotFilter domain.FactorListFilter
	deletedID string
	deletedBy string

	getEntity *domain.FactorEntity
	getErr    error
	listItems []*domain.FactorEntity
	listTotal int64
	createErr error
}

func (f *fakeFactorRepo) Create(_ context.Context, e *domain.FactorEntity) error {
	f.created = e
	return f.createErr
}

func (f *fakeFactorRepo) Update(_ context.Context, e *domain.FactorEntity) error {
	f.updated = e
	return nil
}

func (f *fakeFactorRepo) Delete(_ context.Context, id, deletedBy string) error {
	f.deletedID, f.deletedBy = id, deletedBy
	return nil
}

func (f *fakeFactorRepo) GetByID(_ context.Context, _ string) (*domain.FactorEntity, error) {
	return f.getEntity, f.getErr
}

func (f *fakeFactorRepo) GetByPair(_ context.Context, _, _ string) (*domain.FactorEntity, error) {
	return f.getEntity, f.getErr
}

func (f *fakeFactorRepo) List(_ context.Context, filter domain.FactorListFilter) ([]*domain.FactorEntity, int64, error) {
	f.gotFilter = filter
	return f.listItems, f.listTotal, nil
}

var _ domain.FactorRepository = (*fakeFactorRepo)(nil)

// =============================================================================
// FactorCreateHandler
// =============================================================================

func TestFactorCreateHandler_Handle(t *testing.T) {
	t.Run("success - persists an ordered pair with its operation", func(t *testing.T) {
		repo := &fakeFactorRepo{}
		got, err := app.NewFactorCreateHandler(repo).Handle(context.Background(), app.FactorCreateCommand{
			FromCode: "RND", ToCode: "TBL", Factor: 0.82,
			Operation: domain.OperationDivide, Note: "seed", IsActive: true, CreatedBy: "admin",
		})

		require.NoError(t, err)
		assert.Equal(t, "RND", got.FromCode())
		assert.Equal(t, "TBL", got.ToCode())
		assert.InDelta(t, 0.82, got.Factor(), 1e-9)
		assert.Equal(t, domain.OperationDivide, got.Operation())
		assert.Same(t, got, repo.created)
	})

	t.Run("rejects a self-directed pair", func(t *testing.T) {
		repo := &fakeFactorRepo{}
		_, err := app.NewFactorCreateHandler(repo).Handle(context.Background(), app.FactorCreateCommand{
			FromCode: "RND", ToCode: "RND", Factor: 1, Operation: domain.OperationMultiply, CreatedBy: "admin",
		})

		require.ErrorIs(t, err, domain.ErrFactorSelfPair)
		assert.Nil(t, repo.created)
	})

	t.Run("rejects a non-positive factor", func(t *testing.T) {
		repo := &fakeFactorRepo{}
		_, err := app.NewFactorCreateHandler(repo).Handle(context.Background(), app.FactorCreateCommand{
			FromCode: "RND", ToCode: "TBL", Factor: 0, Operation: domain.OperationMultiply, CreatedBy: "admin",
		})

		require.ErrorIs(t, err, domain.ErrFactorNotPositive)
	})

	t.Run("rejects an unknown operation", func(t *testing.T) {
		repo := &fakeFactorRepo{}
		_, err := app.NewFactorCreateHandler(repo).Handle(context.Background(), app.FactorCreateCommand{
			FromCode: "RND", ToCode: "TBL", Factor: 1, Operation: "ADD", CreatedBy: "admin",
		})

		require.ErrorIs(t, err, domain.ErrFactorInvalidOperation)
	})

	t.Run("propagates a duplicate-pair repo error", func(t *testing.T) {
		repo := &fakeFactorRepo{createErr: domain.ErrFactorAlreadyExists}
		_, err := app.NewFactorCreateHandler(repo).Handle(context.Background(), app.FactorCreateCommand{
			FromCode: "RND", ToCode: "TBL", Factor: 1, Operation: domain.OperationMultiply, CreatedBy: "admin",
		})

		require.ErrorIs(t, err, domain.ErrFactorAlreadyExists)
	})
}

// =============================================================================
// FactorGetHandler / FactorDeleteHandler
// =============================================================================

func TestFactorGetHandler_Handle_NotFound(t *testing.T) {
	repo := &fakeFactorRepo{getErr: domain.ErrFactorNotFound}

	_, err := app.NewFactorGetHandler(repo).Handle(context.Background(), app.FactorGetQuery{ID: "missing"})

	require.ErrorIs(t, err, domain.ErrFactorNotFound)
}

func TestFactorDeleteHandler_Handle(t *testing.T) {
	repo := &fakeFactorRepo{}

	err := app.NewFactorDeleteHandler(repo).Handle(context.Background(), app.FactorDeleteCommand{ID: "id-1", DeletedBy: "admin"})

	require.NoError(t, err)
	assert.Equal(t, "id-1", repo.deletedID)
	assert.Equal(t, "admin", repo.deletedBy)
}

// =============================================================================
// FactorUpdateHandler
// =============================================================================

func TestFactorUpdateHandler_Handle(t *testing.T) {
	live := func() *domain.FactorEntity {
		return domain.ReconstructFactor("id-1", "RND", "TBL", 0.82, domain.OperationDivide, "n", true,
			"t0", "admin", "", "", "", "")
	}

	t.Run("success - the pair stays immutable while the factor flips direction", func(t *testing.T) {
		repo := &fakeFactorRepo{getEntity: live()}

		got, err := app.NewFactorUpdateHandler(repo).Handle(context.Background(), app.FactorUpdateCommand{
			ID: "id-1", Factor: 0.9, Operation: domain.OperationMultiply, Note: "revised",
			IsActive: false, UpdatedBy: "editor",
		})

		require.NoError(t, err)
		assert.Equal(t, "RND", got.FromCode())
		assert.Equal(t, "TBL", got.ToCode())
		assert.InDelta(t, 0.9, got.Factor(), 1e-9)
		assert.Equal(t, domain.OperationMultiply, got.Operation())
		assert.Equal(t, "revised", got.Note())
		assert.False(t, got.IsActive())
		assert.Same(t, got, repo.updated)
	})

	t.Run("rejects a non-positive factor", func(t *testing.T) {
		repo := &fakeFactorRepo{getEntity: live()}

		_, err := app.NewFactorUpdateHandler(repo).Handle(context.Background(), app.FactorUpdateCommand{
			ID: "id-1", Factor: -1, Operation: domain.OperationMultiply, UpdatedBy: "editor",
		})

		require.ErrorIs(t, err, domain.ErrFactorNotPositive)
		assert.Nil(t, repo.updated)
	})

	t.Run("rejects an update on a soft-deleted row", func(t *testing.T) {
		deleted := domain.ReconstructFactor("id-1", "RND", "TBL", 0.82, domain.OperationDivide, "", true,
			"t0", "admin", "", "", "t9", "admin")
		repo := &fakeFactorRepo{getEntity: deleted}

		_, err := app.NewFactorUpdateHandler(repo).Handle(context.Background(), app.FactorUpdateCommand{
			ID: "id-1", Factor: 1, Operation: domain.OperationMultiply, UpdatedBy: "editor",
		})

		require.ErrorIs(t, err, domain.ErrDeleted)
	})
}

// =============================================================================
// FactorListHandler
// =============================================================================

func TestFactorListHandler_Handle(t *testing.T) {
	t.Run("maps the query onto the filter, including the pair filters", func(t *testing.T) {
		repo := &fakeFactorRepo{listTotal: 5}

		got, err := app.NewFactorListHandler(repo).Handle(context.Background(), app.FactorListQuery{
			Page: 1, PageSize: 2, FromCode: "RND", ToCode: "TBL", SortBy: "factor", SortOrder: "desc",
		})

		require.NoError(t, err)
		assert.Equal(t, "RND", repo.gotFilter.FromCode)
		assert.Equal(t, "TBL", repo.gotFilter.ToCode)
		assert.Equal(t, "factor", repo.gotFilter.SortBy)
		assert.Equal(t, int64(5), got.TotalItems)
		assert.Equal(t, int32(3), got.TotalPages, "5 rows over a page size of 2 is 3 pages")
	})

	t.Run("applies filter defaults for a zero-valued query", func(t *testing.T) {
		repo := &fakeFactorRepo{}

		_, err := app.NewFactorListHandler(repo).Handle(context.Background(), app.FactorListQuery{})

		require.NoError(t, err)
		assert.Equal(t, int32(1), repo.gotFilter.Page)
		assert.Equal(t, int32(10), repo.gotFilter.PageSize)
		assert.Equal(t, "from_code", repo.gotFilter.SortBy)
		assert.Equal(t, "asc", repo.gotFilter.SortOrder)
	})
}
