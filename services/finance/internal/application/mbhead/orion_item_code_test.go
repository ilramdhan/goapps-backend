// Package mbhead_test covers the MB head Orion Item Code uniqueness rule: unique for
// values being set or changed, legacy duplicates and empty values left alone.
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
)

func TestCreateHandler_OrionItemCodeUniqueness(t *testing.T) {
	ctx := context.Background()
	mb := newMachineMB(t)

	newRepos := func() (*MockRepository, *MockMachineRepository) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil).Maybe()
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		return repo, mrepo
	}

	t.Run("duplicate is rejected", func(t *testing.T) {
		repo, mrepo := newRepos()
		repo.On("ExistsByOrionItemCode", ctx, "ORI-1", uuid.Nil).Return(true, nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo, nil).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", OrionItemCode: strptr(" ORI-1 "),
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateOrionItemCode)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("free code is trimmed and stored", func(t *testing.T) {
		repo, mrepo := newRepos()
		repo.On("ExistsByOrionItemCode", ctx, "ORI-2", uuid.Nil).Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo, nil).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", OrionItemCode: strptr(" ORI-2 "),
		})

		require.NoError(t, err)
		require.NotNil(t, got.OrionItemCode())
		assert.Equal(t, "ORI-2", *got.OrionItemCode())
	})

	for name, code := range map[string]*string{"absent": nil, "empty": strptr(""), "blank": strptr("  ")} {
		t.Run(name+" code is allowed and never probed", func(t *testing.T) {
			repo, mrepo := newRepos()
			repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

			got, err := mbhead.NewCreateHandler(repo, mrepo, nil).Handle(ctx, mbhead.CreateCommand{
				MBCosting: "MB001", CreatedBy: "admin", OrionItemCode: code,
			})

			require.NoError(t, err)
			assert.Nil(t, got.OrionItemCode(), "blank is stored as NULL")
			repo.AssertNotCalled(t, "ExistsByOrionItemCode", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestUpdateHandler_OrionItemCodeOnlyCheckedWhenChanged(t *testing.T) {
	ctx := context.Background()

	stored := func(t *testing.T, code *string) *mbheaddomain.Entity {
		t.Helper()
		e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "MB001", CreatedBy: "admin", OrionItemCode: code})
		require.NoError(t, err)
		return e
	}

	t.Run("unchanged value is not probed - legacy duplicates stay editable", func(t *testing.T) {
		e := stored(t, strptr("ORI-LEGACY"))
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", OrionItemCode: strptr(" ORI-LEGACY "),
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByOrionItemCode", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("absent value is not probed and keeps the stored code", func(t *testing.T) {
		e := stored(t, strptr("ORI-KEEP"))
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{ID: e.ID(), UpdatedBy: "admin"})

		require.NoError(t, err)
		require.NotNil(t, got.OrionItemCode())
		assert.Equal(t, "ORI-KEEP", *got.OrionItemCode())
		repo.AssertNotCalled(t, "ExistsByOrionItemCode", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("empty value clears without a probe", func(t *testing.T) {
		e := stored(t, strptr("ORI-CLEAR"))
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", OrionItemCode: strptr(""),
		})

		require.NoError(t, err)
		assert.Nil(t, got.OrionItemCode())
		repo.AssertNotCalled(t, "ExistsByOrionItemCode", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("changed value that is taken is rejected", func(t *testing.T) {
		e := stored(t, nil)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("ExistsByOrionItemCode", ctx, "ORI-8", e.ID()).Return(true, nil)

		got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", OrionItemCode: strptr("ORI-8"),
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateOrionItemCode)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("changed value that is free is stored", func(t *testing.T) {
		e := stored(t, strptr("ORI-7"))
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("ExistsByOrionItemCode", ctx, "ORI-9", e.ID()).Return(false, nil)
		repo.On("Update", ctx, e).Return(nil)

		got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", OrionItemCode: strptr("ORI-9"),
		})

		require.NoError(t, err)
		assert.Equal(t, "ORI-9", *got.OrionItemCode())
	})
}
