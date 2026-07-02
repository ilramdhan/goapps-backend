package costproductmaster_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
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
