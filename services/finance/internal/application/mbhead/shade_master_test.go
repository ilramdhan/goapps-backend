package mbhead_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// MockShadeRepository is a mock implementation of shade.Repository. Only GetByCode
// and Create carry behavior — the two calls the shade auto-create depends on.
type MockShadeRepository struct {
	mock.Mock
}

func (m *MockShadeRepository) Create(ctx context.Context, e *shade.Shade) error {
	return m.Called(ctx, e).Error(0)
}

func (m *MockShadeRepository) GetByID(_ context.Context, _ int64) (*shade.Shade, error) {
	panic("not used")
}

func (m *MockShadeRepository) GetByCode(ctx context.Context, code string) (*shade.Shade, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*shade.Shade), args.Error(1)
}

func (m *MockShadeRepository) List(_ context.Context, _ shade.ListFilter) ([]*shade.Shade, int64, error) {
	panic("not used")
}

func (m *MockShadeRepository) Update(_ context.Context, _ *shade.Shade) error { panic("not used") }

func (m *MockShadeRepository) UpsertSourced(_ context.Context, _ shade.Sourced) (shade.UpsertOutcome, error) {
	panic("not used")
}

func TestUpdateHandler_ShadeMasterAutoCreate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		mbhCode    *string
		shadeName  *string
		lookupRet  *shade.Shade
		lookupErr  error
		createErr  error
		wantCreate bool
		wantCode   string
		wantName   string
	}{
		{name: "missing shade - created MANUAL, code upper-cased", mbhCode: strptr(" z114s "), shadeName: strptr("Navy"),
			lookupErr: shade.ErrNotFound, wantCreate: true, wantCode: "Z114S", wantName: "Navy"},
		{name: "missing shade without name - name falls back to code", mbhCode: strptr("NL"),
			lookupErr: shade.ErrNotFound, wantCreate: true, wantCode: "NL", wantName: "NL"},
		{name: "existing shade - skipped", mbhCode: strptr("NL"), lookupRet: mustShade(t, "NL")},
		{name: "empty shade code - no lookup", mbhCode: strptr("  ")},
		{name: "create fails - save still succeeds", mbhCode: strptr("NL"),
			lookupErr: shade.ErrNotFound, createErr: errors.New("db down"), wantCreate: true, wantCode: "NL", wantName: "NL"},
		{name: "lookup fails - save still succeeds, no create", mbhCode: strptr("NL"), lookupErr: errors.New("db down")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepository)
			srepo := new(MockShadeRepository)
			id := uuid.New()
			repo.On("GetByID", ctx, id).Return(newHeadWithCosting(t, "MB001"), nil)
			repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
			if tt.lookupRet != nil || tt.lookupErr != nil {
				srepo.On("GetByCode", ctx, mock.Anything).Return(tt.lookupRet, tt.lookupErr)
			}
			if tt.wantCreate {
				srepo.On("Create", ctx, mock.MatchedBy(func(s *shade.Shade) bool {
					return s.Code() == tt.wantCode && s.Name() == tt.wantName &&
						s.Source() == shade.SourceManual && s.CreatedBy() == "editor"
				})).Return(tt.createErr)
			}

			got, err := mbhead.NewUpdateHandler(repo, srepo).Handle(ctx, mbhead.UpdateCommand{
				ID: id, MBHCode: tt.mbhCode, ShadeName: tt.shadeName, UpdatedBy: "editor",
			})

			require.NoError(t, err)
			require.NotNil(t, got)
			if !tt.wantCreate {
				srepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
			}
			srepo.AssertExpectations(t)
		})
	}
}

func TestCreateHandler_ShadeMasterAutoCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("missing shade - created after head is persisted", func(t *testing.T) {
		repo := new(MockRepository)
		srepo := new(MockShadeRepository)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		srepo.On("GetByCode", ctx, "Z114S").Return(nil, shade.ErrNotFound)
		srepo.On("Create", ctx, mock.MatchedBy(func(s *shade.Shade) bool {
			return s.Code() == "Z114S" && s.Name() == "Navy" && s.CreatedBy() == "admin"
		})).Return(nil)

		got, err := mbhead.NewCreateHandler(repo, newMBMachineRepo(), srepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", MBHCode: strptr("z114s"), ShadeName: "Navy", CreatedBy: "admin",
		})

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "z114s", *got.MBHCode(), "stored head value is left as sent")
		srepo.AssertExpectations(t)
	})

	t.Run("existing shade - skipped", func(t *testing.T) {
		repo := new(MockRepository)
		srepo := new(MockShadeRepository)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		srepo.On("GetByCode", ctx, "NL").Return(mustShade(t, "NL"), nil)

		_, err := mbhead.NewCreateHandler(repo, newMBMachineRepo(), srepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", MBHCode: strptr("NL"), CreatedBy: "admin",
		})

		require.NoError(t, err)
		srepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("head create fails - no shade work", func(t *testing.T) {
		repo := new(MockRepository)
		srepo := new(MockShadeRepository)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(errors.New("boom"))

		_, err := mbhead.NewCreateHandler(repo, newMBMachineRepo(), srepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", MBHCode: strptr("NL"), CreatedBy: "admin",
		})

		require.Error(t, err)
		srepo.AssertNotCalled(t, "GetByCode", mock.Anything, mock.Anything)
	})

	t.Run("falls back to shade_code when mbh_code empty", func(t *testing.T) {
		repo := new(MockRepository)
		srepo := new(MockShadeRepository)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		srepo.On("GetByCode", ctx, "AB1").Return(mustShade(t, "AB1"), nil)

		_, err := mbhead.NewCreateHandler(repo, newMBMachineRepo(), srepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", ShadeCode: "ab1", CreatedBy: "admin",
		})

		require.NoError(t, err)
		srepo.AssertExpectations(t)
	})
}

func mustShade(t *testing.T, code string) *shade.Shade {
	t.Helper()
	s, err := shade.New(shade.NewParams{Code: code, Name: code, CreatedBy: "seed"})
	require.NoError(t, err)
	return s
}
