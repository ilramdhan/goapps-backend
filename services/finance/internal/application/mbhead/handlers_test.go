// Package mbhead_test provides unit tests for MB Head application layer handlers.
package mbhead_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	mbparamdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// noOfProcessOptions mirrors the live mst_mb_param_option rows for NO_OF_PROCESS
// (S=1, D=2, T=3, all active) plus one deactivated option, so the membership check
// can be exercised without hardcoding the set in production code.
func noOfProcessOptions(t *testing.T, includeInactive bool) []*mbparamdomain.Option {
	t.Helper()
	specs := []struct {
		code, numeric string
		active        bool
	}{
		{"S", "1", true},
		{"D", "2", true},
		{"T", "3", true},
	}
	if includeInactive {
		specs = append(specs, struct {
			code, numeric string
			active        bool
		}{"Q", "4", false})
	}
	out := make([]*mbparamdomain.Option, 0, len(specs))
	for i, s := range specs {
		out = append(out, mbparamdomain.ReconstructOption(
			uuid.NewString(), "NO_OF_PROCESS", s.code, s.numeric, "", int32(i+1), s.active,
		))
	}
	return out
}

// newNoOfProcessParamRepo returns a param repository stubbed with the live NO_OF_PROCESS
// option set. GetByCode is Maybe() because handlers that fail earlier never reach the check.
func newNoOfProcessParamRepo(t *testing.T) *MockParamRepository {
	t.Helper()
	return newNoOfProcessParamRepoWith(t, noOfProcessOptions(t, true))
}

func newNoOfProcessParamRepoWith(t *testing.T, options []*mbparamdomain.Option) *MockParamRepository {
	t.Helper()
	param, err := mbparamdomain.NewEntity(
		"NO_OF_PROCESS", "No of Process", mbparamdomain.TypePicklist, "", "", "D", "", 1, "seed",
	)
	require.NoError(t, err)
	param.SetOptions(options)

	repo := new(MockParamRepository)
	repo.On("GetByCode", mock.Anything, "NO_OF_PROCESS").Return(param, nil).Maybe()
	return repo
}

// MockRepository is a mock implementation of mbhead.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, entity *mbheaddomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbheaddomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbheaddomain.Entity), args.Error(1)
}

func (m *MockRepository) GetByMBCosting(ctx context.Context, mbCosting string) (*mbheaddomain.Entity, error) {
	args := m.Called(ctx, mbCosting)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbheaddomain.Entity), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter mbheaddomain.ListFilter) ([]*mbheaddomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*mbheaddomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, entity *mbheaddomain.Entity) error {
	args := m.Called(ctx, entity)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error) {
	args := m.Called(ctx, mbCosting)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) UpdateEntryStatus(ctx context.Context, id uuid.UUID, entryStatus string, currentVersion int32, stateReason string) error {
	args := m.Called(ctx, id, entryStatus, currentVersion, stateReason)
	return args.Error(0)
}

func (m *MockRepository) ListAll(ctx context.Context, filter mbheaddomain.ExportFilter) ([]*mbheaddomain.Entity, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*mbheaddomain.Entity), args.Error(1)
}

func (m *MockRepository) Transition(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *mbheaddomain.ParamSnapshot) error {
	args := m.Called(ctx, id, fromState, toState, currentVersion, stateReason, actorUserID, params)
	return args.Error(0)
}

func (m *MockRepository) TransitionWithAutoGen(ctx context.Context, id uuid.UUID, fromState, toState string, currentVersion int32, stateReason, actorUserID string, params *mbheaddomain.ParamSnapshot, entity *mbheaddomain.Entity) error {
	args := m.Called(ctx, id, fromState, toState, currentVersion, stateReason, actorUserID, params, entity)
	return args.Error(0)
}

func (m *MockRepository) RefreezeCostParams(ctx context.Context, id uuid.UUID, entity *mbheaddomain.Entity, params *mbheaddomain.ParamSnapshot) error {
	args := m.Called(ctx, id, entity, params)
	return args.Error(0)
}

func (m *MockRepository) ExistsByDevCode(ctx context.Context, code string, excludeID *uuid.UUID) (bool, error) {
	args := m.Called(ctx, code, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByVsNumber(ctx context.Context, number string, excludeID *uuid.UUID) (bool, error) {
	args := m.Called(ctx, number, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ListShades(ctx context.Context, mbhID uuid.UUID) ([]*mbheaddomain.Shade, error) {
	args := m.Called(ctx, mbhID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*mbheaddomain.Shade), args.Error(1)
}

func (m *MockRepository) ReplaceShades(ctx context.Context, mbhID uuid.UUID, shades []*mbheaddomain.Shade, actorUserID string) error {
	args := m.Called(ctx, mbhID, shades, actorUserID)
	return args.Error(0)
}

func (m *MockRepository) ListShadesByHeads(ctx context.Context, mbhIDs []uuid.UUID) (map[uuid.UUID][]*mbheaddomain.Shade, error) {
	args := m.Called(ctx, mbhIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[uuid.UUID][]*mbheaddomain.Shade), args.Error(1)
}

// newTestEntity builds a domain entity with every field required by spec section 2.1 populated,
// so tests exercise handler wiring rather than re-testing domain validation.
func newTestEntity(t *testing.T, mbCosting string) *mbheaddomain.Entity {
	t.Helper()
	e, err := mbheaddomain.New(mbheaddomain.NewInput{
		MBCosting:    mbCosting,
		MgtName:      "MGT NAME",
		DevCode:      "DEV-" + mbCosting,
		VsNumber:     "VS-" + mbCosting,
		NoOfProcess:  "S",
		ShadeCode:    "SH1",
		ShadeName:    "SHADE ONE",
		CrossSection: "ROUND",
		FinalProduct: "FP",
		Denier:       150,
		Filament:     48,
		LdrPrsn:      10,
		LustureCode:  "BR",
		CreatedBy:    "admin",
	})
	require.NoError(t, err)
	return e
}

// validCreateCommand mirrors newTestEntity as an application-layer command.
func validCreateCommand(mbCosting string) mbhead.CreateCommand {
	return mbhead.CreateCommand{
		MBCosting:       mbCosting,
		MgtName:         "MGT NAME",
		DevCode:         "DEV-" + mbCosting,
		VsNumber:        "VS-" + mbCosting,
		NoOfProcess:     "S",
		ShadeCode:       "SH1",
		ShadeName:       "SHADE ONE",
		CrossSection:    "ROUND",
		MBHFinalProduct: "FP",
		Denier:          150,
		Filament:        48,
		MBHLdrPrsn:      10,
		LustureCode:     "BR",
		CreatedBy:       "admin",
	}
}

func TestCreateHandler_Handle(t *testing.T) {
	t.Run("success - creates new MB Head", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		mockRepo.On("ExistsByDevCode", ctx, "DEV-MB001", (*uuid.UUID)(nil)).Return(false, nil)
		mockRepo.On("ExistsByVsNumber", ctx, "VS-MB001", (*uuid.UUID)(nil)).Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate dev code", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		mockRepo.On("ExistsByDevCode", ctx, "DEV-MB001", (*uuid.UUID)(nil)).Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrDevCodeAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - duplicate vs number", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		mockRepo.On("ExistsByDevCode", ctx, "DEV-MB001", (*uuid.UUID)(nil)).Return(false, nil)
		mockRepo.On("ExistsByVsNumber", ctx, "VS-MB001", (*uuid.UUID)(nil)).Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrVsNumberAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - missing required vs number", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")
		cmd.VsNumber = ""

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrEmptyVsNumber)
	})

	t.Run("error - duplicate mb_costing", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(true, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrAlreadyExists)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - empty mb_costing", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("")

		mockRepo.On("ExistsByMBCosting", ctx, "").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrEmptyMBCosting)
	})

	t.Run("error - empty created_by", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB002")
		cmd.CreatedBy = ""

		mockRepo.On("ExistsByMBCosting", ctx, "MB002").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrEmptyCreatedBy)
	})
}

func TestGetHandler_Handle(t *testing.T) {
	t.Run("success - returns entity by ID", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		expected := newTestEntity(t, "MB001")

		mockRepo.On("GetByID", ctx, id).Return(expected, nil)

		query := mbhead.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbheaddomain.ErrNotFound)

		query := mbhead.GetQuery{ID: id}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - nil UUID returns not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewGetHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("GetByID", ctx, uuid.Nil).Return(nil, mbheaddomain.ErrNotFound)

		query := mbhead.GetQuery{ID: uuid.Nil}
		result, err := handler.Handle(ctx, query)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
	})
}

func TestUpdateHandler_Handle(t *testing.T) {
	t.Run("success - updates entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewUpdateHandler(mockRepo, mockParams)
		ctx := context.Background()

		id := uuid.New()
		entity := newTestEntity(t, "MB001")

		newCosting := "MB001-UPD"
		cmd := mbhead.UpdateCommand{
			ID:              id,
			MBCosting:       &newCosting,
			MgtName:         "MGT NAME",
			DevCode:         "DEV-MB001",
			VsNumber:        "VS-MB001",
			NoOfProcess:     "S",
			ShadeCode:       "SH1",
			ShadeName:       "SHADE ONE",
			CrossSection:    "ROUND",
			MBHFinalProduct: "FP",
			Denier:          150,
			Filament:        48,
			MBHLdrPrsn:      10,
			UpdatedBy:       "admin",
		}

		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("ExistsByDevCode", ctx, "DEV-MB001", &id).Return(false, nil)
		mockRepo.On("ExistsByVsNumber", ctx, "VS-MB001", &id).Return(false, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		mockRepo.On("ReplaceShades", ctx, entity.ID(), mock.Anything, "admin").Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "MB001-UPD", result.MBCosting())
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewUpdateHandler(mockRepo, mockParams)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("GetByID", ctx, id).Return(nil, mbheaddomain.ErrNotFound)

		cmd := mbhead.UpdateCommand{ID: id, UpdatedBy: "admin"}
		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestDeleteHandler_Handle(t *testing.T) {
	t.Run("success - soft deletes entity", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(nil)

		cmd := mbhead.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - not found", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewDeleteHandler(mockRepo)
		ctx := context.Background()

		id := uuid.New()
		mockRepo.On("SoftDelete", ctx, id, "admin").Return(mbheaddomain.ErrNotFound)

		cmd := mbhead.DeleteCommand{ID: id, DeletedBy: "admin"}
		err := handler.Handle(ctx, cmd)

		assert.ErrorIs(t, err, mbheaddomain.ErrNotFound)
		mockRepo.AssertExpectations(t)
	})
}

func TestListHandler_Handle(t *testing.T) {
	t.Run("success - returns paginated list", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		entity1 := newTestEntity(t, "MB001")
		entity2 := newTestEntity(t, "MB002")

		mockRepo.On("List", ctx, mock.AnythingOfType("mbhead.ListFilter")).Return(
			[]*mbheaddomain.Entity{entity1, entity2},
			int64(2),
			nil,
		)

		query := mbhead.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(1), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})

	t.Run("success - empty result", func(t *testing.T) {
		mockRepo := new(MockRepository)
		handler := mbhead.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("List", ctx, mock.AnythingOfType("mbhead.ListFilter")).Return(
			[]*mbheaddomain.Entity{},
			int64(0),
			nil,
		)

		query := mbhead.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		assert.Empty(t, result.Items)
		assert.Equal(t, int64(0), result.TotalItems)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

// TestNoOfProcessMembership covers the mbh_no_of_process membership check against the live
// mst_mb_param_option set (spec section 2.3). The permitted set is never hardcoded in
// production code, so these tests drive it entirely from the stubbed option rows.
func TestNoOfProcessMembership(t *testing.T) {
	t.Run("create - rejects a code with no option row", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")
		cmd.NoOfProcess = "X"

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrInvalidNoOfProcess)
		mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("create - rejects a deactivated option", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")
		cmd.NoOfProcess = "Q" // present in mst_mb_param_option but is_active = false

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrInvalidNoOfProcess)
		mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("create - accepts an option added to the master without a code change", func(t *testing.T) {
		mockRepo := new(MockRepository)
		extended := append(noOfProcessOptions(t, false), mbparamdomain.ReconstructOption(
			uuid.NewString(), "NO_OF_PROCESS", "Z", "4", "", 4, true,
		))
		mockParams := newNoOfProcessParamRepoWith(t, extended)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")
		cmd.NoOfProcess = "Z"

		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		mockRepo.On("ExistsByDevCode", ctx, "DEV-MB001", (*uuid.UUID)(nil)).Return(false, nil)
		mockRepo.On("ExistsByVsNumber", ctx, "VS-MB001", (*uuid.UUID)(nil)).Return(false, nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Z", result.NoOfProcess())
		mockRepo.AssertExpectations(t)
	})

	t.Run("create - surfaces a param repository failure", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := new(MockParamRepository)
		mockParams.On("GetByCode", mock.Anything, "NO_OF_PROCESS").
			Return(nil, mbparamdomain.ErrNotFound)
		handler := mbhead.NewCreateHandler(mockRepo, mockParams)
		ctx := context.Background()

		cmd := validCreateCommand("MB001")
		mockRepo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbparamdomain.ErrNotFound)
		mockRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("update - rejects a code with no option row", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewUpdateHandler(mockRepo, mockParams)
		ctx := context.Background()

		id := uuid.New()
		entity := newTestEntity(t, "MB001")
		mockRepo.On("GetByID", ctx, id).Return(entity, nil)

		cmd := validUpdateCommand(id)
		cmd.NoOfProcess = "X"

		result, err := handler.Handle(ctx, cmd)

		assert.Nil(t, result)
		assert.ErrorIs(t, err, mbheaddomain.ErrInvalidNoOfProcess)
		mockRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
		mockRepo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("update - accepts a live option", func(t *testing.T) {
		mockRepo := new(MockRepository)
		mockParams := newNoOfProcessParamRepo(t)
		handler := mbhead.NewUpdateHandler(mockRepo, mockParams)
		ctx := context.Background()

		id := uuid.New()
		entity := newTestEntity(t, "MB001")

		cmd := validUpdateCommand(id)
		cmd.NoOfProcess = "T"

		mockRepo.On("GetByID", ctx, id).Return(entity, nil)
		mockRepo.On("ExistsByDevCode", ctx, cmd.DevCode, &id).Return(false, nil)
		mockRepo.On("ExistsByVsNumber", ctx, cmd.VsNumber, &id).Return(false, nil)
		mockRepo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		mockRepo.On("ReplaceShades", ctx, entity.ID(), mock.Anything, "admin").Return(nil)

		result, err := handler.Handle(ctx, cmd)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "T", result.NoOfProcess())
		mockRepo.AssertExpectations(t)
	})
}

// validUpdateCommand mirrors validCreateCommand for the update path.
func validUpdateCommand(id uuid.UUID) mbhead.UpdateCommand {
	return mbhead.UpdateCommand{
		ID:              id,
		MgtName:         "MGT NAME",
		DevCode:         "DEV-MB001",
		VsNumber:        "VS-MB001",
		NoOfProcess:     "S",
		ShadeCode:       "SH1",
		ShadeName:       "SHADE ONE",
		CrossSection:    "ROUND",
		MBHFinalProduct: "FP",
		Denier:          150,
		Filament:        48,
		MBHLdrPrsn:      10,
		UpdatedBy:       "admin",
	}
}
