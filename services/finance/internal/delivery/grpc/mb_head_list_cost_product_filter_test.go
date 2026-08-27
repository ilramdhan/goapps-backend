package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// captureListMBHeadRepo embeds fakeFrozenMBHeadRepo (defined in
// mb_head_frozen_check_status_test.go, same package) to get a full mbhead.Repository for
// free, and shadows only List to capture the filter that ListMBHeads hands down — proving
// the gRPC layer's zero-means-unset mapping for the new cost_product_id request field
// (R16), matching the existing product_type_id convention on
// ListCostProductMastersRequest.
type captureListMBHeadRepo struct {
	fakeFrozenMBHeadRepo
	lastFilter mbhead.ListFilter
}

func (c *captureListMBHeadRepo) List(_ context.Context, filter mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	c.lastFilter = filter
	return nil, 0, nil
}

// TestMBHeadHandler_ListMBHeads_CostProductIDFilter covers both regression cases required
// for R16 at the delivery layer: an absent/zero request field must leave the repository
// filter's CostProductID nil (never accidentally "filter on cost_product_id = 0"), and a
// non-zero value must be forwarded verbatim.
func TestMBHeadHandler_ListMBHeads_CostProductIDFilter(t *testing.T) {
	t.Run("zero value means no filter", func(t *testing.T) {
		repo := &captureListMBHeadRepo{}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)

		_, err = h.ListMBHeads(context.Background(), &financev1.ListMBHeadsRequest{Page: 1, PageSize: 10})
		require.NoError(t, err)
		require.Nil(t, repo.lastFilter.CostProductID)
	})

	t.Run("non-zero value is forwarded to the repository filter", func(t *testing.T) {
		repo := &captureListMBHeadRepo{}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)

		_, err = h.ListMBHeads(context.Background(), &financev1.ListMBHeadsRequest{
			Page: 1, PageSize: 10, CostProductId: 987,
		})
		require.NoError(t, err)
		require.NotNil(t, repo.lastFilter.CostProductID)
		require.Equal(t, int64(987), *repo.lastFilter.CostProductID)
	})
}
