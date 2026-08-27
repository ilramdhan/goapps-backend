package costproductmaster_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

// =============================================================================
// fakeRepo — in-memory test double for domain.Repository. Captures the filter
// passed to List so mapping tests can assert on it.
// =============================================================================
type fakeRepo struct {
	gotFilter domain.Filter
	listItems []*domain.CostProductMaster
	listTotal int64
	listErr   error
}

func (f *fakeRepo) Create(_ context.Context, _ *domain.CostProductMaster) error { return nil }

func (f *fakeRepo) GetBySysID(_ context.Context, _ int64) (*domain.CostProductMaster, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeRepo) GetByCode(_ context.Context, _ string) (*domain.CostProductMaster, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeRepo) Update(_ context.Context, _ *domain.CostProductMaster) error { return nil }

func (f *fakeRepo) List(_ context.Context, filter domain.Filter) ([]*domain.CostProductMaster, int64, error) {
	f.gotFilter = filter
	return f.listItems, f.listTotal, f.listErr
}

func (f *fakeRepo) BulkCreate(_ context.Context, _ []*domain.CostProductMaster, _ string) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (f *fakeRepo) ListAll(_ context.Context, filter domain.Filter) ([]*domain.CostProductMaster, error) {
	f.gotFilter = filter
	return f.listItems, nil
}

func (f *fakeRepo) BulkUpsertByLegacyID(_ context.Context, _ []domain.ProductUpsertInput, _ string) ([]domain.ProductUpsertResult, error) {
	return []domain.ProductUpsertResult{}, nil
}

func (f *fakeRepo) ListAllLegacyIDs(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (f *fakeRepo) RollbackImport(_ context.Context, _ []int64) error { return nil }

func (f *fakeRepo) UnlockWithLog(_ context.Context, _ domain.LockLogInput) error { return nil }

var _ domain.Repository = (*fakeRepo)(nil)

// =============================================================================
// ListHandler — query → filter mapping
// =============================================================================

func TestListHandler_Handle_MapsQueryToFilter(t *testing.T) {
	tests := []struct {
		name  string
		query app.ListQuery
		want  domain.Filter
	}{
		{
			name: "multi-type filter and legacy single type are both forwarded",
			query: app.ListQuery{
				Search:         "ZZORA9001",
				ProductTypeID:  2,
				ProductTypeIDs: []int32{3, 5},
				ActiveFilter:   "active",
				Page:           1,
				PageSize:       20,
			},
			want: domain.Filter{
				Search:         "ZZORA9001",
				ProductTypeID:  2,
				ProductTypeIDs: []int32{3, 5},
				ActiveFilter:   "active",
				Page:           1,
				PageSize:       20,
			},
		},
		{
			name: "new sort keys pass through unchanged",
			query: app.ListQuery{
				SortBy:    "oracle_sys_id",
				SortOrder: "desc",
				Page:      2,
				PageSize:  10,
			},
			want: domain.Filter{
				SortBy:    "oracle_sys_id",
				SortOrder: "desc",
				Page:      2,
				PageSize:  10,
			},
		},
		{
			name:  "empty query maps to zero filter",
			query: app.ListQuery{},
			want:  domain.Filter{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{listTotal: 42}
			h := app.NewListHandler(repo)

			res, err := h.Handle(context.Background(), tt.query)

			require.NoError(t, err)
			assert.Equal(t, tt.want, repo.gotFilter)
			assert.Equal(t, int64(42), res.Total)
			assert.Empty(t, res.Items)
		})
	}
}

// =============================================================================
// CreateHandler — guard E2: MB products are never manually creatable.
//
// MB-typed products exist only as the auto-generated output of the MB Recipe
// workflow (cpm_source = 'MB_RECIPE', written at Validate time). A product-master
// create of type MB would produce a product with no recipe behind it.
// =============================================================================

// fakeTypeRepo is an in-memory costproducttype.Repository resolving typeID → typeCode.
type fakeTypeRepo struct {
	byID map[int32]string
}

func (f *fakeTypeRepo) Create(_ context.Context, _ *cptdomain.CostProductType) error { return nil }

func (f *fakeTypeRepo) GetByID(_ context.Context, id int32) (*cptdomain.CostProductType, error) {
	code, ok := f.byID[id]
	if !ok {
		return nil, cptdomain.ErrNotFound
	}
	return cptdomain.Reconstruct(id, code, code, true, time.Time{}, time.Time{}), nil
}

func (f *fakeTypeRepo) GetByCode(_ context.Context, _ string) (*cptdomain.CostProductType, error) {
	return nil, cptdomain.ErrNotFound
}

func (f *fakeTypeRepo) Update(_ context.Context, _ *cptdomain.CostProductType) error { return nil }

func (f *fakeTypeRepo) List(_ context.Context, _ cptdomain.Filter) ([]*cptdomain.CostProductType, int64, error) {
	return nil, 0, nil
}

func (f *fakeTypeRepo) ListAllActive(_ context.Context) ([]*cptdomain.CostProductType, error) {
	return nil, nil
}

var _ cptdomain.Repository = (*fakeTypeRepo)(nil)

// createTrackingRepo records whether Create ever reached the repository.
type createTrackingRepo struct {
	fakeRepo
	createCalled bool
}

func (r *createTrackingRepo) Create(_ context.Context, _ *domain.CostProductMaster) error {
	r.createCalled = true
	return nil
}

func TestCreateHandler_Handle_RejectsMBProductType(t *testing.T) {
	const mbTypeID, yarnTypeID = 7, 2
	types := &fakeTypeRepo{byID: map[int32]string{mbTypeID: "MB", yarnTypeID: "YARN"}}

	t.Run("MB type is refused and never reaches the repository", func(t *testing.T) {
		repo := &createTrackingRepo{}
		h := app.NewCreateHandler(repo, types)

		got, err := h.Handle(context.Background(), app.CreateCommand{
			ProductTypeID: mbTypeID,
			ProductName:   "MB Red 001",
			GradeCode:     "AX",
			ActorUserID:   "admin",
		})

		require.ErrorIs(t, err, domain.ErrMBProductNotManuallyCreatable)
		assert.Nil(t, got)
		assert.False(t, repo.createCalled, "an MB product must never be written")
	})

	t.Run("a non-MB type still creates normally", func(t *testing.T) {
		repo := &createTrackingRepo{}
		h := app.NewCreateHandler(repo, types)

		got, err := h.Handle(context.Background(), app.CreateCommand{
			ProductTypeID: yarnTypeID,
			ProductName:   "Yarn 150/48",
			GradeCode:     "AX",
			ActorUserID:   "admin",
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, repo.createCalled)
	})

	t.Run("an unresolvable type falls through to the normal create path", func(t *testing.T) {
		repo := &createTrackingRepo{}
		h := app.NewCreateHandler(repo, types)

		_, err := h.Handle(context.Background(), app.CreateCommand{
			ProductTypeID: 999,
			ProductName:   "Unknown type",
			GradeCode:     "AX",
			ActorUserID:   "admin",
		})

		require.NoError(t, err, "an unknown type is the FK's problem, not this guard's")
		assert.True(t, repo.createCalled)
	})
}
