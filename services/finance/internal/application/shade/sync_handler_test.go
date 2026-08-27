// Package shade_test provides unit tests for shade application handlers.
package shade_test

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appshade "github.com/mutugading/goapps-backend/services/finance/internal/application/shade"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// MockRepository is a mock implementation of shade.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *shade.Shade) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id int64) (*shade.Shade, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shade.Shade), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code string) (*shade.Shade, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shade.Shade), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter shade.ListFilter) ([]*shade.Shade, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*shade.Shade), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *shade.Shade) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) UpsertSourced(ctx context.Context, src shade.Sourced) (shade.UpsertOutcome, error) {
	args := m.Called(ctx, src)
	return args.Get(0).(shade.UpsertOutcome), args.Error(1)
}

// MockSource is a mock implementation of shade.Source.
type MockSource struct {
	mock.Mock
}

func (m *MockSource) ListShades(ctx context.Context) ([]shade.Sourced, error) {
	args := m.Called(ctx)
	return args.Get(0).([]shade.Sourced), args.Error(1)
}

func TestSyncHandler_Execute_TalliesInsertedUpdatedSkipped(t *testing.T) {
	oracleRepo := new(MockSource)
	pgRepo := new(MockRepository)

	items := []shade.Sourced{
		{Code: "NEW1"},    // will be inserted
		{Code: "EXIST1"},  // will be updated
		{Code: "MANUAL1"}, // manually-owned row -> skipped by the repository
	}
	oracleRepo.On("ListShades", mock.Anything).Return(items, nil)
	pgRepo.On("UpsertSourced", mock.Anything, items[0]).Return(shade.OutcomeInserted, nil)
	pgRepo.On("UpsertSourced", mock.Anything, items[1]).Return(shade.OutcomeUpdated, nil)
	pgRepo.On("UpsertSourced", mock.Anything, items[2]).Return(shade.OutcomeSkipped, nil)

	handler := appshade.NewSyncHandler(oracleRepo, pgRepo, zerolog.Nop())
	result, err := handler.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalRows)
	assert.Equal(t, 1, result.Inserted)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, 1, result.Skipped)
	oracleRepo.AssertExpectations(t)
	pgRepo.AssertExpectations(t)
}

func TestSyncHandler_Execute_NoOracleSource_ReturnsErrSyncNotConfigured(t *testing.T) {
	pgRepo := new(MockRepository)
	handler := appshade.NewSyncHandler(nil, pgRepo, zerolog.Nop())

	_, err := handler.Execute(context.Background())

	require.ErrorIs(t, err, shade.ErrSyncNotConfigured)
	pgRepo.AssertNotCalled(t, "UpsertSourced", mock.Anything, mock.Anything)
}

func TestSyncHandler_Execute_UpsertError_PropagatesAndStops(t *testing.T) {
	oracleRepo := new(MockSource)
	pgRepo := new(MockRepository)

	items := []shade.Sourced{{Code: "BAD1"}}
	oracleRepo.On("ListShades", mock.Anything).Return(items, nil)
	pgRepo.On("UpsertSourced", mock.Anything, items[0]).Return(shade.OutcomeSkipped, assert.AnError)

	handler := appshade.NewSyncHandler(oracleRepo, pgRepo, zerolog.Nop())
	_, err := handler.Execute(context.Background())

	require.Error(t, err)
}
