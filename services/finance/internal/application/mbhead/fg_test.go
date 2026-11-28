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
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

type fakeFgReader struct {
	poy      *mbspin.FgProduct
	err      error
	filled   []string
	fillCall int
}

func (f *fakeFgReader) GetFgProduct(_ context.Context, _ int64) (*mbspin.FgProduct, error) {
	return f.poy, f.err
}

func (f *fakeFgReader) FillEmptyFgShade(_ context.Context, _ int64, shadeCode, _ string) (bool, error) {
	f.fillCall++
	f.filled = append(f.filled, shadeCode)
	return true, nil
}

func poy(shadeCode *string) *mbspin.FgProduct {
	return &mbspin.FgProduct{ID: 7, Code: "POY-1", Name: "Poy", ErpItemCode: strPtr(" ERP-7 "), ShadeCode: shadeCode, TypeCode: "POY", IsActive: true}
}

func TestCreateHandler_Fg(t *testing.T) {
	ctx := context.Background()
	newCmd := func() mbhead.CreateCommand {
		return mbhead.CreateCommand{MBCosting: "MB-FG", ShadeCode: "s1", CreatedBy: "admin", FgProductID: 7}
	}
	setup := func(r *fakeFgReader, created bool) (*mbhead.CreateHandler, *MockRepository) {
		repo := new(MockRepository)
		if created {
			repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		}
		return mbhead.NewCreateHandler(repo, newMBMachineRepo(), nil).WithFgReader(r), repo
	}

	t.Run("POY with same shade links and snapshots item code", func(t *testing.T) {
		r := &fakeFgReader{poy: poy(strPtr(" S1 "))}
		h, repo := setup(r, true)
		e, err := h.Handle(ctx, newCmd())
		require.NoError(t, err)
		require.NotNil(t, e.FgProductID())
		assert.Equal(t, int64(7), *e.FgProductID())
		assert.Equal(t, "ERP-7", *e.FgItemCode())
		assert.Zero(t, r.fillCall)
		repo.AssertExpectations(t)
	})

	t.Run("empty POY shade is filled with recipe shade", func(t *testing.T) {
		r := &fakeFgReader{poy: poy(nil)}
		h, _ := setup(r, true)
		_, err := h.Handle(ctx, newCmd())
		require.NoError(t, err)
		assert.Equal(t, []string{"s1"}, r.filled)
	})

	t.Run("non POY is rejected", func(t *testing.T) {
		p := poy(nil)
		p.TypeCode = "FDY"
		h, _ := setup(&fakeFgReader{poy: p}, false)
		_, err := h.Handle(ctx, newCmd())
		assert.ErrorIs(t, err, mbspin.ErrFgNotPoy)
	})

	t.Run("shade mismatch is rejected", func(t *testing.T) {
		h, _ := setup(&fakeFgReader{poy: poy(strPtr("OTHER"))}, false)
		_, err := h.Handle(ctx, newCmd())
		assert.ErrorIs(t, err, mbspin.ErrFgShadeMismatch)
	})

	t.Run("mbh_code differing from shade code is rejected", func(t *testing.T) {
		h, _ := setup(&fakeFgReader{poy: poy(nil)}, false)
		cmd := newCmd()
		code := "OTHER"
		cmd.MBHCode = &code
		_, err := h.Handle(ctx, cmd)
		assert.ErrorIs(t, err, mbspin.ErrFgShadeMismatch)
	})

	t.Run("blank recipe shade is rejected", func(t *testing.T) {
		h, _ := setup(&fakeFgReader{poy: poy(nil)}, false)
		cmd := newCmd()
		cmd.ShadeCode = ""
		_, err := h.Handle(ctx, cmd)
		assert.ErrorIs(t, err, mbspin.ErrFgShadeMismatch)
	})

	t.Run("no FG behaves as before without a reader", func(t *testing.T) {
		repo := new(MockRepository)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		cmd := newCmd()
		cmd.FgProductID = 0
		e, err := mbhead.NewCreateHandler(repo, newMBMachineRepo(), nil).Handle(ctx, cmd)
		require.NoError(t, err)
		assert.Nil(t, e.FgProductID())
	})
}

func TestUpdateHandler_Fg(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	build := func(t *testing.T, withFg bool) *mbheaddomain.Entity {
		e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "MB1", ShadeCode: "S1", CreatedBy: "a"})
		require.NoError(t, err)
		if withFg {
			pid := int64(3)
			require.NoError(t, e.SetFg(&pid, nil))
		}
		return e
	}
	run := func(t *testing.T, e *mbheaddomain.Entity, fg *int64, r *fakeFgReader) (*mbheaddomain.Entity, error) {
		repo := new(MockRepository)
		repo.On("GetByID", ctx, id).Return(e, nil)
		repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		return mbhead.NewUpdateHandler(repo, nil).WithFgReader(r).Handle(ctx, mbhead.UpdateCommand{ID: id, UpdatedBy: "a", FgProductID: fg})
	}

	t.Run("nil leaves FG unchanged", func(t *testing.T) {
		got, err := run(t, build(t, true), nil, &fakeFgReader{})
		require.NoError(t, err)
		require.NotNil(t, got.FgProductID())
		assert.Equal(t, int64(3), *got.FgProductID())
	})
	t.Run("zero clears FG", func(t *testing.T) {
		zero := int64(0)
		got, err := run(t, build(t, true), &zero, &fakeFgReader{})
		require.NoError(t, err)
		assert.Nil(t, got.FgProductID())
	})
	t.Run("positive sets FG", func(t *testing.T) {
		v := int64(7)
		got, err := run(t, build(t, false), &v, &fakeFgReader{poy: poy(strPtr("s1"))})
		require.NoError(t, err)
		assert.Equal(t, int64(7), *got.FgProductID())
	})
	t.Run("shade change with existing FG and nil FgProductID mismatches", func(t *testing.T) {
		sh := "NEW"
		repo := new(MockRepository)
		repo.On("GetByID", ctx, id).Return(build(t, true), nil)
		_, err := mbhead.NewUpdateHandler(repo, nil).WithFgReader(&fakeFgReader{poy: poy(strPtr("S1"))}).
			Handle(ctx, mbhead.UpdateCommand{ID: id, UpdatedBy: "a", ShadeCode: &sh})
		assert.ErrorIs(t, err, mbspin.ErrFgShadeMismatch)
	})
	t.Run("shade change with existing FG and empty POY shade fills", func(t *testing.T) {
		sh := "NEW"
		r := &fakeFgReader{poy: poy(nil)}
		repo := new(MockRepository)
		repo.On("GetByID", ctx, id).Return(build(t, true), nil)
		repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		_, err := mbhead.NewUpdateHandler(repo, nil).WithFgReader(r).
			Handle(ctx, mbhead.UpdateCommand{ID: id, UpdatedBy: "a", ShadeCode: &sh})
		require.NoError(t, err)
		assert.Equal(t, []string{"NEW"}, r.filled)
	})
	t.Run("shade shape mismatch rejected", func(t *testing.T) {
		v := int64(7)
		_, err := run(t, build(t, false), &v, &fakeFgReader{poy: poy(strPtr("zzz"))})
		assert.ErrorIs(t, err, mbspin.ErrFgShadeMismatch)
	})
}
