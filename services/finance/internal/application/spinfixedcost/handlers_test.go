// Package spinfixedcost_test provides unit tests for Spin Fixed Cost application layer handlers.
package spinfixedcost_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/spinfixedcost"
	sfcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

const testActor = "admin"

// errRepo is an opaque infrastructure failure used to prove propagation.
var errRepo = errors.New("connection refused")

// MockRepository is a mock implementation of spinfixedcost.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *sfcdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*sfcdomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sfcdomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByPeriod(ctx context.Context, period string) (*sfcdomain.Entity, error) {
	args := m.Called(ctx, period)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sfcdomain.Entity), args.Error(1)
}

func (m *MockRepository) List(
	ctx context.Context, filter sfcdomain.ListFilter,
) ([]*sfcdomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*sfcdomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *sfcdomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByPeriod(ctx context.Context, period string) (bool, error) {
	args := m.Called(ctx, period)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) LoadAnchorStats(
	ctx context.Context, excludeID uuid.UUID,
) (sfcdomain.AnchorStats, error) {
	args := m.Called(ctx, excludeID)
	return args.Get(0).(sfcdomain.AnchorStats), args.Error(1)
}

// methodOrder returns the recorded mock method names in invocation order.
func methodOrder(m *MockRepository) []string {
	names := make([]string, 0, len(m.Calls))
	for _, c := range m.Calls {
		names = append(names, c.Method)
	}
	return names
}

func f64(v float64) *float64 { return &v }
func boolPtr(v bool) *bool   { return &v }

// liveEntity builds an active, non-deleted row for handler tests.
func liveEntity(t *testing.T, period string) *sfcdomain.Entity {
	t.Helper()
	entity, err := sfcdomain.New(sfcdomain.NewInput{
		Period:             period,
		CommonPoyDenier:    150,
		PoyProduction:      1_200_000,
		SpinPowerMonth:     500_000_000,
		SpinManpowerMonth:  200_000_000,
		SpinOverheadsMonth: 100_000_000,
		SpinConssprsMonth:  50_000_000,
		CreatedBy:          testActor,
	})
	require.NoError(t, err)
	return entity
}

// safeStats describes a table where the candidate is a middle row: the guard passes.
func safeStats() sfcdomain.AnchorStats {
	return sfcdomain.AnchorStats{
		RemainingActiveCount:          3,
		EarliestRemainingActivePeriod: "202601",
		HasLiveRowAfterCandidate:      true,
	}
}

func validCreateCommand() spinfixedcost.CreateCommand {
	return spinfixedcost.CreateCommand{
		Period:             "202606",
		CommonPoyDenier:    150,
		PoyProduction:      1_200_000,
		SpinPowerMonth:     500_000_000,
		SpinManpowerMonth:  200_000_000,
		SpinOverheadsMonth: 100_000_000,
		SpinConssprsMonth:  50_000_000,
		CreatedBy:          testActor,
	}
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ExistsByPeriod", ctx, "202606").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

		result, err := handler.Handle(ctx, validCreateCommand())

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "202606", result.Period())
		assert.InDelta(t, 150.0, result.CommonPoyDenier(), 0.001)
		assert.InDelta(t, 1_200_000.0, result.PoyProduction(), 0.001)
		assert.True(t, result.IsActive())
		assert.Equal(t, testActor, result.CreatedBy())
		mockRepo.AssertExpectations(t)
	})

	// The duplicate pre-check exists to turn a raw 23505 into a friendly 409, so it has
	// to run before the insert is attempted, not alongside it.
	t.Run("success - checks ExistsByPeriod before Create", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ExistsByPeriod", ctx, "202606").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

		_, err := handler.Handle(ctx, validCreateCommand())

		require.NoError(t, err)
		assert.Equal(t, []string{"ExistsByPeriod", "Create"}, methodOrder(mockRepo))
	})

	t.Run("error - duplicate period", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ExistsByPeriod", ctx, "202606").Return(true, nil)

		result, err := handler.Handle(ctx, validCreateCommand())

		assert.Nil(t, result)
		assert.ErrorIs(t, err, sfcdomain.ErrDuplicatePeriod)
		mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - ExistsByPeriod failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ExistsByPeriod", ctx, "202606").Return(false, errRepo)

		result, err := handler.Handle(ctx, validCreateCommand())

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("error - Create failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewCreateHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ExistsByPeriod", ctx, "202606").Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(errRepo)

		result, err := handler.Handle(ctx, validCreateCommand())

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - domain validation rejects before Create", func(t *testing.T) {
		tests := []struct {
			name    string
			mutate  func(*spinfixedcost.CreateCommand)
			wantErr error
		}{
			{
				name:    "invalid period",
				mutate:  func(c *spinfixedcost.CreateCommand) { c.Period = "2026" },
				wantErr: sfcdomain.ErrInvalidPeriod,
			},
			{
				name:    "zero denier",
				mutate:  func(c *spinfixedcost.CreateCommand) { c.CommonPoyDenier = 0 },
				wantErr: sfcdomain.ErrNonPositiveDenier,
			},
			{
				name:    "zero production",
				mutate:  func(c *spinfixedcost.CreateCommand) { c.PoyProduction = 0 },
				wantErr: sfcdomain.ErrNonPositiveProduction,
			},
			{
				name:    "negative amount",
				mutate:  func(c *spinfixedcost.CreateCommand) { c.SpinPowerMonth = -1 },
				wantErr: sfcdomain.ErrNegativeAmount,
			},
			{
				name:    "empty created_by",
				mutate:  func(c *spinfixedcost.CreateCommand) { c.CreatedBy = "" },
				wantErr: sfcdomain.ErrEmptyCreatedBy,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				mockRepo := new(MockRepository)
				handler := spinfixedcost.NewCreateHandler(mockRepo)
				ctx := context.Background()

				cmd := validCreateCommand()
				tt.mutate(&cmd)
				mockRepo.On("ExistsByPeriod", ctx, cmd.Period).Return(false, nil)

				result, err := handler.Handle(ctx, cmd)

				assert.Nil(t, result)
				assert.ErrorIs(t, err, tt.wantErr)
				mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			})
		}
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns entity by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		result, err := handler.Handle(ctx, spinfixedcost.GetQuery{ID: id})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "202606", result.Period())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, sfcdomain.ErrNotFound)

		result, err := handler.Handle(ctx, spinfixedcost.GetQuery{ID: id})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, sfcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewGetHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, uuid.Nil).Return(nil, sfcdomain.ErrNotFound)

		result, err := handler.Handle(ctx, spinfixedcost.GetQuery{ID: uuid.Nil})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, sfcdomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates values without touching is_active", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:              id,
			CommonPoyDenier: f64(167),
			SpinPowerMonth:  f64(600_000_000),
			UpdatedBy:       "editor",
		})

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.InDelta(t, 167.0, result.CommonPoyDenier(), 0.001)
		assert.InDelta(t, 600_000_000.0, result.SpinPowerMonth(), 0.001)
		assert.Equal(t, "202606", result.Period())
		mockRepo.AssertExpectations(t)
	})

	// Deactivating is the only mutation that can strip an anchor; the guard is a DB
	// round-trip, so it must not fire on ordinary value edits.
	t.Run("guard - not consulted when the row stays active", func(t *testing.T) {
		cases := []struct {
			name     string
			isActive *bool
		}{
			{"is_active omitted", nil},
			{"is_active explicitly true", boolPtr(true)},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mockRepo := new(MockRepository)
				handler := spinfixedcost.NewUpdateHandler(mockRepo)
				ctx := context.Background()

				id := uuid.New()
				mockRepo.On("GetByID", ctx, id).Return(liveEntity(t, "202606"), nil)
				mockRepo.On("Update", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

				result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
					ID:             id,
					SpinPowerMonth: f64(1),
					IsActive:       tc.isActive,
					UpdatedBy:      "editor",
				})

				require.NoError(t, err)
				assert.True(t, result.IsActive())
				mockRepo.AssertNotCalled(t, "LoadAnchorStats", mock.Anything, mock.Anything)
				assert.Equal(t, []string{"GetByID", "Update"}, methodOrder(mockRepo))
			})
		}
	})

	t.Run("guard - consulted when the update deactivates the row", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("LoadAnchorStats", ctx, entity.ID()).Return(safeStats(), nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:        id,
			IsActive:  boolPtr(false),
			UpdatedBy: "editor",
		})

		require.NoError(t, err)
		assert.False(t, result.IsActive())
		assert.Equal(t, []string{"GetByID", "LoadAnchorStats", "Update"}, methodOrder(mockRepo))
		mockRepo.AssertExpectations(t)
	})

	t.Run("guard - violation aborts before repo.Update", func(t *testing.T) {
		cases := []struct {
			name    string
			stats   sfcdomain.AnchorStats
			wantErr error
		}{
			{
				name: "only active row",
				stats: sfcdomain.AnchorStats{
					RemainingActiveCount:          0,
					EarliestRemainingActivePeriod: "",
					HasLiveRowAfterCandidate:      false,
				},
				wantErr: sfcdomain.ErrAnchorRowOnly,
			},
			{
				name: "earliest active row with later rows",
				stats: sfcdomain.AnchorStats{
					RemainingActiveCount:          2,
					EarliestRemainingActivePeriod: "202610",
					HasLiveRowAfterCandidate:      true,
				},
				wantErr: sfcdomain.ErrAnchorRowEarliest,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mockRepo := new(MockRepository)
				handler := spinfixedcost.NewUpdateHandler(mockRepo)
				ctx := context.Background()

				id := uuid.New()
				entity := liveEntity(t, "202609")
				mockRepo.On("GetByID", ctx, id).Return(entity, nil)
				mockRepo.On("LoadAnchorStats", ctx, entity.ID()).Return(tc.stats, nil)

				result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
					ID:        id,
					IsActive:  boolPtr(false),
					UpdatedBy: "editor",
				})

				assert.Nil(t, result)
				assert.ErrorIs(t, err, tc.wantErr)
				mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
				// The in-memory entity must not have been mutated either.
				assert.True(t, entity.IsActive())
				assert.Nil(t, entity.UpdatedBy())
			})
		}
	})

	t.Run("guard - deactivating an already inactive row is allowed", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		inactive := sfcdomain.Reconstruct(sfcdomain.ReconstructInput{
			ID:              id,
			Period:          "202601",
			CommonPoyDenier: 150,
			PoyProduction:   1000,
			IsActive:        false,
			CreatedAt:       time.Now(),
			CreatedBy:       testActor,
		})
		mockRepo.On("GetByID", ctx, id).Return(inactive, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(nil)

		_, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:        id,
			IsActive:  boolPtr(false),
			UpdatedBy: "editor",
		})

		require.NoError(t, err)
		mockRepo.AssertNotCalled(t, "LoadAnchorStats", mock.Anything, mock.Anything)
	})

	t.Run("error - LoadAnchorStats failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("LoadAnchorStats", ctx, entity.ID()).
			Return(sfcdomain.AnchorStats{}, errRepo)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:        id,
			IsActive:  boolPtr(false),
			UpdatedBy: "editor",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, sfcdomain.ErrNotFound)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{ID: id, UpdatedBy: "editor"})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, sfcdomain.ErrNotFound)
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - domain validation rejects before repo.Update", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(liveEntity(t, "202606"), nil)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:              id,
			CommonPoyDenier: f64(0),
			UpdatedBy:       "editor",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, sfcdomain.ErrNonPositiveDenier)
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("error - repo.Update failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewUpdateHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(liveEntity(t, "202606"), nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*spinfixedcost.Entity")).Return(errRepo)

		result, err := handler.Handle(ctx, spinfixedcost.UpdateCommand{
			ID:             id,
			SpinPowerMonth: f64(1),
			UpdatedBy:      "editor",
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes and consults the guard first", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("LoadAnchorStats", ctx, entity.ID()).Return(safeStats(), nil)
		mockRepo.On("SoftDelete", ctx, id, "remover").Return(nil)

		err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

		require.NoError(t, err)
		assert.Equal(t, []string{"GetByID", "LoadAnchorStats", "SoftDelete"}, methodOrder(mockRepo))
		mockRepo.AssertExpectations(t)
	})

	// Unlike Update, delete always removes the row from period resolution, so the guard
	// runs unconditionally — including for rows the guard will ultimately wave through.
	t.Run("guard - always consulted, even for an inactive row", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		inactive := sfcdomain.Reconstruct(sfcdomain.ReconstructInput{
			ID:              id,
			Period:          "202601",
			CommonPoyDenier: 150,
			PoyProduction:   1000,
			IsActive:        false,
			CreatedAt:       time.Now(),
			CreatedBy:       testActor,
		})
		mockRepo.On("GetByID", ctx, id).Return(inactive, nil)
		mockRepo.On("LoadAnchorStats", ctx, id).
			Return(sfcdomain.AnchorStats{RemainingActiveCount: 0}, nil)
		mockRepo.On("SoftDelete", ctx, id, "remover").Return(nil)

		err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

		require.NoError(t, err)
		mockRepo.AssertCalled(t, "LoadAnchorStats", ctx, id)
		mockRepo.AssertExpectations(t)
	})

	t.Run("guard - violation aborts before repo.SoftDelete", func(t *testing.T) {
		cases := []struct {
			name    string
			period  string
			stats   sfcdomain.AnchorStats
			wantErr error
		}{
			{
				name:   "only active row",
				period: "202606",
				stats: sfcdomain.AnchorStats{
					RemainingActiveCount:          0,
					EarliestRemainingActivePeriod: "",
					HasLiveRowAfterCandidate:      false,
				},
				wantErr: sfcdomain.ErrAnchorRowOnly,
			},
			{
				name:   "earliest active row with later rows",
				period: "202609",
				stats: sfcdomain.AnchorStats{
					RemainingActiveCount:          1,
					EarliestRemainingActivePeriod: "202610",
					HasLiveRowAfterCandidate:      true,
				},
				wantErr: sfcdomain.ErrAnchorRowEarliest,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mockRepo := new(MockRepository)
				handler := spinfixedcost.NewDeleteHandler(mockRepo)
				ctx := context.Background()

				id := uuid.New()
				entity := liveEntity(t, tc.period)
				mockRepo.On("GetByID", ctx, id).Return(entity, nil)
				mockRepo.On("LoadAnchorStats", ctx, entity.ID()).Return(tc.stats, nil)

				err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

				assert.ErrorIs(t, err, tc.wantErr)
				mockRepo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
				assert.False(t, entity.IsDeleted())
			})
		}
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, sfcdomain.ErrNotFound)

		err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

		assert.ErrorIs(t, err, sfcdomain.ErrNotFound)
		mockRepo.AssertNotCalled(t, "LoadAnchorStats", mock.Anything, mock.Anything)
		mockRepo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("error - LoadAnchorStats failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("LoadAnchorStats", ctx, entity.ID()).
			Return(sfcdomain.AnchorStats{}, errRepo)

		err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("error - repo.SoftDelete failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		entity := liveEntity(t, "202606")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("LoadAnchorStats", ctx, entity.ID()).Return(safeStats(), nil)
		mockRepo.On("SoftDelete", ctx, id, "remover").Return(errRepo)

		err := handler.Handle(ctx, spinfixedcost.DeleteCommand{ID: id, DeletedBy: "remover"})

		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		items := []*sfcdomain.Entity{liveEntity(t, "202606"), liveEntity(t, "202607")}
		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Return(items, int64(2), nil)

		result, err := handler.Handle(ctx, spinfixedcost.ListQuery{Page: 1, PageSize: 10})

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - empty result has zero total pages", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Return([]*sfcdomain.Entity{}, int64(0), nil)

		result, err := handler.Handle(ctx, spinfixedcost.ListQuery{Page: 1, PageSize: 10})

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Equal(t, int64(0), result.TotalItems)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - every filter field reaches the repository", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		active := true
		var captured sfcdomain.ListFilter
		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Run(func(args mock.Arguments) {
				captured = args.Get(1).(sfcdomain.ListFilter)
			}).
			Return([]*sfcdomain.Entity{}, int64(0), nil)

		_, err := handler.Handle(ctx, spinfixedcost.ListQuery{
			Page:      2,
			PageSize:  25,
			Search:    "spin",
			Period:    "202606",
			IsActive:  &active,
			SortBy:    "created_at",
			SortOrder: "asc",
		})

		require.NoError(t, err)
		assert.Equal(t, "spin", captured.Search)
		assert.Equal(t, "202606", captured.Period)
		require.NotNil(t, captured.IsActive)
		assert.True(t, *captured.IsActive)
		assert.Equal(t, 2, captured.Page)
		assert.Equal(t, 25, captured.PageSize)
		assert.Equal(t, "created_at", captured.SortBy)
		assert.Equal(t, "asc", captured.SortOrder)
		assert.Equal(t, 25, captured.Offset())
	})

	t.Run("success - filter defaults are applied before the repository call", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		var captured sfcdomain.ListFilter
		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Run(func(args mock.Arguments) {
				captured = args.Get(1).(sfcdomain.ListFilter)
			}).
			Return([]*sfcdomain.Entity{}, int64(0), nil)

		result, err := handler.Handle(ctx, spinfixedcost.ListQuery{Page: 0, PageSize: 5000})

		require.NoError(t, err)
		assert.Equal(t, 1, captured.Page)
		assert.Equal(t, 100, captured.PageSize)
		assert.Equal(t, "period", captured.SortBy)
		assert.Equal(t, "desc", captured.SortOrder)
		assert.Nil(t, captured.IsActive)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(100), result.PageSize)
	})

	t.Run("success - total pages round up on a partial last page", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Return([]*sfcdomain.Entity{}, int64(23), nil)

		result, err := handler.Handle(ctx, spinfixedcost.ListQuery{Page: 1, PageSize: 10})

		require.NoError(t, err)
		assert.Equal(t, int64(23), result.TotalItems)
		assert.Equal(t, int32(3), result.TotalPages)
	})

	t.Run("error - repository failure propagates", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := spinfixedcost.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("spinfixedcost.ListFilter")).
			Return(nil, int64(0), errRepo)

		result, err := handler.Handle(ctx, spinfixedcost.ListQuery{Page: 1, PageSize: 10})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, errRepo)
		mockRepo.AssertExpectations(t)
	})
}
