package rmgroup_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// mockRepo is a testify/mock implementation of rmgroup.Repository.
type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) CreateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockRepo) GetHeadByID(ctx context.Context, id uuid.UUID) (*rmgroup.Head, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockRepo) GetHeadByCode(ctx context.Context, code rmgroup.Code) (*rmgroup.Head, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Head), args.Error(1)
}

func (m *mockRepo) ListHeads(ctx context.Context, filter rmgroup.ListFilter) ([]*rmgroup.Head, int64, error) {
	args := m.Called(ctx, filter)
	var heads []*rmgroup.Head
	if v := args.Get(0); v != nil {
		heads = v.([]*rmgroup.Head)
	}
	return heads, args.Get(1).(int64), args.Error(2)
}

func (m *mockRepo) UpdateHead(ctx context.Context, head *rmgroup.Head) error {
	return m.Called(ctx, head).Error(0)
}

func (m *mockRepo) ListAllHeads(ctx context.Context, activeFilter *bool) ([]*rmgroup.Head, error) {
	args := m.Called(ctx, activeFilter)
	var heads []*rmgroup.Head
	if v := args.Get(0); v != nil {
		heads = v.([]*rmgroup.Head)
	}
	return heads, args.Error(1)
}

func (m *mockRepo) SoftDeleteHead(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

func (m *mockRepo) ExistsHeadByCode(ctx context.Context, code rmgroup.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepo) ExistsHeadByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepo) AddDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockRepo) UpdateDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return m.Called(ctx, detail).Error(0)
}

func (m *mockRepo) GetDetailByID(ctx context.Context, id uuid.UUID) (*rmgroup.Detail, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockRepo) GetActiveDetailByItemCodeGrade(ctx context.Context, itemCode rmgroup.ItemCode, gradeCode string) (*rmgroup.Detail, error) {
	args := m.Called(ctx, itemCode, gradeCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rmgroup.Detail), args.Error(1)
}

func (m *mockRepo) ListDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockRepo) ListActiveDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	args := m.Called(ctx, headID)
	var out []*rmgroup.Detail
	if v := args.Get(0); v != nil {
		out = v.([]*rmgroup.Detail)
	}
	return out, args.Error(1)
}

func (m *mockRepo) SoftDeleteDetail(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

// mockUngroupedReader mocks appgroup.UngroupedItemsReader.
type mockUngroupedReader struct {
	mock.Mock
}

func (m *mockUngroupedReader) ListUngroupedItems(ctx context.Context, filter appgroup.UngroupedItemsFilter) ([]*syncdata.ItemConsStockPO, int64, error) {
	args := m.Called(ctx, filter)
	var items []*syncdata.ItemConsStockPO
	if v := args.Get(0); v != nil {
		items = v.([]*syncdata.ItemConsStockPO)
	}
	return items, args.Get(1).(int64), args.Error(2)
}
