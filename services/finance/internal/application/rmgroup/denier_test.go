package rmgroup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

func f64(v float64) *float64 { return &v }

func TestHead_SetDenier(t *testing.T) {
	head := newHead(t)
	assert.Nil(t, head.Denier())
	require.NoError(t, head.SetDenier(f64(40)))
	require.NotNil(t, head.Denier())
	assert.InDelta(t, 40, *head.Denier(), 1e-9)
	assert.ErrorIs(t, head.SetDenier(f64(0)), rmgroup.ErrInvalidDenier)
	assert.ErrorIs(t, head.SetDenier(f64(-1)), rmgroup.ErrInvalidDenier)
	require.NoError(t, head.SetDenier(nil))
	assert.Nil(t, head.Denier())
}

func TestCreateHandler_Denier(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		denier  *float64
		want    *float64
		wantErr error
	}{
		{"absent is NULL", nil, nil, nil},
		{"zero is NULL", f64(0), nil, nil},
		{"positive set", f64(70), f64(70), nil},
		{"negative rejected", f64(-5), nil, rmgroup.ErrInvalidDenier},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := new(mockRepo)
			repo.On("ExistsHeadByCode", ctx, mock.AnythingOfType("rmgroup.Code")).Return(false, nil)
			repo.On("CreateHead", ctx, mock.Anything).Return(nil).Maybe()
			head, err := appgroup.NewCreateHandler(repo).Handle(ctx, appgroup.CreateCommand{
				Code: "SPX-1", Name: "Spandex", CreatedBy: "u", Denier: tc.denier,
			})
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.want == nil {
				assert.Nil(t, head.Denier())
			} else {
				require.NotNil(t, head.Denier())
				assert.InDelta(t, *tc.want, *head.Denier(), 1e-9)
			}
		})
	}
}

func TestUpdateHandler_Denier_FlagOnlyPath(t *testing.T) {
	ctx := context.Background()
	run := func(t *testing.T, initial, patch *float64) (*rmgroup.Head, error) {
		head := newHead(t)
		require.NoError(t, head.SetDenier(initial))
		repo := new(mockRepo)
		repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
		repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound).Maybe()
		repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound).Maybe()
		repo.On("UpdateHead", ctx, mock.Anything).Return(nil).Maybe()
		_, err := appgroup.NewUpdateHandler(repo).Handle(ctx, appgroup.UpdateCommand{
			HeadID: head.ID().String(), Period: "202503", Denier: patch, UpdatedBy: "u",
		})
		return head, err
	}

	t.Run("absent leaves unchanged", func(t *testing.T) {
		head, err := run(t, f64(20), nil)
		require.NoError(t, err)
		require.NotNil(t, head.Denier())
		assert.InDelta(t, 20, *head.Denier(), 1e-9)
	})
	t.Run("positive sets", func(t *testing.T) {
		head, err := run(t, nil, f64(40))
		require.NoError(t, err)
		require.NotNil(t, head.Denier())
		assert.InDelta(t, 40, *head.Denier(), 1e-9)
	})
	t.Run("zero clears", func(t *testing.T) {
		head, err := run(t, f64(40), f64(0))
		require.NoError(t, err)
		assert.Nil(t, head.Denier())
	})
	t.Run("negative rejected", func(t *testing.T) {
		_, err := run(t, nil, f64(-1))
		assert.ErrorIs(t, err, rmgroup.ErrInvalidDenier)
	})
}

func TestCreateHandler_DenierUseDraft(t *testing.T) {
	ctx := context.Background()
	for _, want := range []bool{false, true} {
		repo := new(mockRepo)
		repo.On("ExistsHeadByCode", ctx, mock.AnythingOfType("rmgroup.Code")).Return(false, nil)
		repo.On("CreateHead", ctx, mock.Anything).Return(nil)
		head, err := appgroup.NewCreateHandler(repo).Handle(ctx, appgroup.CreateCommand{
			Code: "SPX-1", Name: "Spandex", CreatedBy: "u", Denier: f64(40), DenierUseDraft: want,
		})
		require.NoError(t, err)
		assert.Equal(t, want, head.DenierUseDraft())
	}
}

func TestUpdateHandler_DenierUseDraft_FlagOnlyPath(t *testing.T) {
	ctx := context.Background()
	b := func(v bool) *bool { return &v }
	run := func(t *testing.T, initial bool, patch *bool) (*rmgroup.Head, *mockRepo) {
		head := newHead(t)
		head.SetDenierUseDraft(initial)
		repo := new(mockRepo)
		repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
		repo.On("GetHeadPeriodSnapshot", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound).Maybe()
		repo.On("GetLatestHeadPeriodSnapshotBefore", ctx, head.ID(), "202503").Return(nil, rmgroup.ErrNotFound).Maybe()
		repo.On("UpdateHead", ctx, mock.Anything).Return(nil).Maybe()
		_, err := appgroup.NewUpdateHandler(repo).Handle(ctx, appgroup.UpdateCommand{
			HeadID: head.ID().String(), Period: "202503", DenierUseDraft: patch, UpdatedBy: "u",
		})
		require.NoError(t, err)
		return head, repo
	}

	t.Run("absent leaves unchanged", func(t *testing.T) {
		head, repo := run(t, true, nil)
		assert.True(t, head.DenierUseDraft())
		repo.AssertNotCalled(t, "UpdateHead", ctx, mock.Anything)
	})
	t.Run("true sets", func(t *testing.T) {
		head, repo := run(t, false, b(true))
		assert.True(t, head.DenierUseDraft())
		repo.AssertCalled(t, "UpdateHead", ctx, mock.Anything)
	})
	t.Run("false clears", func(t *testing.T) {
		head, _ := run(t, true, b(false))
		assert.False(t, head.DenierUseDraft())
	})
}
