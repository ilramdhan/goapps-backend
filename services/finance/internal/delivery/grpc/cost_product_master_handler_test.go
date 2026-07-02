package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// fakeCPMRepo is an in-memory test double for domain.Repository that captures
// the filter passed to List.
type fakeCPMRepo struct {
	gotFilter domain.Filter
}

func (f *fakeCPMRepo) Create(_ context.Context, _ *domain.CostProductMaster) error { return nil }

func (f *fakeCPMRepo) GetBySysID(_ context.Context, _ int64) (*domain.CostProductMaster, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeCPMRepo) GetByCode(_ context.Context, _ string) (*domain.CostProductMaster, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeCPMRepo) Update(_ context.Context, _ *domain.CostProductMaster) error { return nil }

func (f *fakeCPMRepo) List(_ context.Context, filter domain.Filter) ([]*domain.CostProductMaster, int64, error) {
	f.gotFilter = filter
	return []*domain.CostProductMaster{}, 0, nil
}

func (f *fakeCPMRepo) BulkCreate(_ context.Context, _ []*domain.CostProductMaster, _ string) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (f *fakeCPMRepo) ListAll(_ context.Context, filter domain.Filter) ([]*domain.CostProductMaster, error) {
	f.gotFilter = filter
	return []*domain.CostProductMaster{}, nil
}

func (f *fakeCPMRepo) BulkUpsertByLegacyID(_ context.Context, _ []domain.ProductUpsertInput, _ string) ([]domain.ProductUpsertResult, error) {
	return []domain.ProductUpsertResult{}, nil
}

func (f *fakeCPMRepo) ListAllLegacyIDs(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (f *fakeCPMRepo) RollbackImport(_ context.Context, _ []int64) error { return nil }

var _ domain.Repository = (*fakeCPMRepo)(nil)

func newCPMHandlerForTest(t *testing.T) (*CostProductMasterHandler, *fakeCPMRepo) {
	t.Helper()
	repo := &fakeCPMRepo{}
	h, err := NewCostProductMasterHandler(repo)
	require.NoError(t, err)
	return h, repo
}

func TestListCostProductMasters_MapsRequestToFilter(t *testing.T) {
	h, repo := newCPMHandlerForTest(t)

	resp, err := h.ListCostProductMasters(context.Background(), &financev1.ListCostProductMastersRequest{
		Search:         "ZZORA9001",
		ProductTypeId:  2,
		ProductTypeIds: []int32{3, 5},
		ActiveFilter:   "active",
		Pagination:     &commonv1.PaginationRequest{Page: 1, PageSize: 20},
		SortBy:         "oracle_sys_id",
		SortOrder:      "desc",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.True(t, resp.GetBase().GetIsSuccess(), "expected success, got: %s", resp.GetBase().GetMessage())
	assert.Equal(t, "ZZORA9001", repo.gotFilter.Search)
	assert.Equal(t, int32(2), repo.gotFilter.ProductTypeID)
	assert.Equal(t, []int32{3, 5}, repo.gotFilter.ProductTypeIDs)
	assert.Equal(t, "oracle_sys_id", repo.gotFilter.SortBy)
	assert.Equal(t, "desc", repo.gotFilter.SortOrder)
}

func TestListCostProductMasters_NewSortKeysPassValidation(t *testing.T) {
	sortKeys := []string{
		"",
		"product_code",
		"product_name",
		"created_at",
		"updated_at",
		"product_type_code",
		"shade_code",
		"grade_code",
		"oracle_sys_id",
		"erp_compound_key",
		"type_label",
		"status",
	}
	for _, key := range sortKeys {
		t.Run("sort_by="+key, func(t *testing.T) {
			h, repo := newCPMHandlerForTest(t)

			resp, err := h.ListCostProductMasters(context.Background(), &financev1.ListCostProductMastersRequest{
				SortBy: key,
			})

			require.NoError(t, err)
			assert.True(t, resp.GetBase().GetIsSuccess(), "sort_by %q should pass validation: %s", key, resp.GetBase().GetMessage())
			assert.Equal(t, key, repo.gotFilter.SortBy)
		})
	}
}

func TestListCostProductMasters_InvalidSortKeyRejected(t *testing.T) {
	h, _ := newCPMHandlerForTest(t)

	resp, err := h.ListCostProductMasters(context.Background(), &financev1.ListCostProductMastersRequest{
		SortBy: "bogus_column",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.GetBase())
	assert.False(t, resp.GetBase().GetIsSuccess())
}
