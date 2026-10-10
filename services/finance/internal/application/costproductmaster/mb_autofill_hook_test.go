package costproductmaster_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

type spyFiller struct{ calls int }

func (s *spyFiller) BestEffort(context.Context, []int64, string) { s.calls++ }

type failingCreateRepo struct{ fakeRepo }

func (failingCreateRepo) Create(context.Context, *domain.CostProductMaster) error {
	return errors.New("insert failed")
}

func TestCreateHandler_MBSourceAutoFillHook(t *testing.T) {
	cmd := app.CreateCommand{ProductTypeID: 2, ProductName: "Yarn", GradeCode: "AX", ShadeCode: "MC-1", ActorUserID: "u"}

	t.Run("runs once after a successful create", func(t *testing.T) {
		spy := &spyFiller{}
		h := app.NewCreateHandler(&createTrackingRepo{}, &fakeTypeRepo{byID: map[int32]string{2: "YARN"}}).WithMBSourceAutoFill(spy)
		_, err := h.Handle(context.Background(), cmd)
		require.NoError(t, err)
		assert.Equal(t, 1, spy.calls)
	})

	t.Run("does not run when create fails", func(t *testing.T) {
		spy := &spyFiller{}
		h := app.NewCreateHandler(&failingCreateRepo{}, &fakeTypeRepo{byID: map[int32]string{2: "YARN"}}).WithMBSourceAutoFill(spy)
		_, err := h.Handle(context.Background(), cmd)
		require.Error(t, err)
		assert.Zero(t, spy.calls)
	})

	t.Run("nil filler keeps legacy behaviour", func(t *testing.T) {
		h := app.NewCreateHandler(&createTrackingRepo{}, &fakeTypeRepo{byID: map[int32]string{2: "YARN"}})
		_, err := h.Handle(context.Background(), cmd)
		require.NoError(t, err)
	})
}
