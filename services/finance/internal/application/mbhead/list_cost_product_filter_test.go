// Package mbhead_test provides unit tests for MB Head application layer handlers.
package mbhead_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestListHandler_Handle_CostProductIDFilter pins R16's application-layer contract: the
// ListQuery.CostProductID pointer must flow through untouched into the repository filter,
// and an omitted CostProductID must reach the repository as nil — never a zero value that
// could be mistaken for "filter on cost_product_id = 0".
func TestListHandler_Handle_CostProductIDFilter(t *testing.T) {
	t.Run("set - passed through to repository filter", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		id := int64(123)
		mockRepo.On("List", ctx, mock.MatchedBy(func(f mbheaddomain.ListFilter) bool {
			return f.CostProductID != nil && *f.CostProductID == id
		})).Return([]*mbheaddomain.Entity{}, int64(0), nil)

		query := mbhead.ListQuery{Page: 1, PageSize: 10, CostProductID: &id}
		_, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("unset - repository filter stays nil, behaves like before R16", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.MatchedBy(func(f mbheaddomain.ListFilter) bool {
			return f.CostProductID == nil
		})).Return([]*mbheaddomain.Entity{}, int64(0), nil)

		query := mbhead.ListQuery{Page: 1, PageSize: 10}
		_, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})
}
