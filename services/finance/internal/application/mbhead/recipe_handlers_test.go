// Package mbhead_test provides unit tests for the MB recipe fields (P5) added to
// the MB Head create/update handlers.
package mbhead_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// --- machine.Repository test double -------------------------------------------------

// MockMachineRepository is a mock implementation of machine.Repository. Only
// GetByCode carries behavior — it is the single method B5 depends on.
type MockMachineRepository struct {
	mock.Mock
}

func (m *MockMachineRepository) Create(ctx context.Context, entity *machine.Entity) error {
	return m.Called(ctx, entity).Error(0)
}

func (m *MockMachineRepository) GetByID(ctx context.Context, id uuid.UUID) (*machine.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*machine.Entity), args.Error(1)
}

func (m *MockMachineRepository) GetByCode(ctx context.Context, code string) (*machine.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*machine.Entity), args.Error(1)
}

func (m *MockMachineRepository) List(ctx context.Context, filter machine.ListFilter) ([]*machine.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*machine.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockMachineRepository) Update(ctx context.Context, entity *machine.Entity) error {
	return m.Called(ctx, entity).Error(0)
}

func (m *MockMachineRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return m.Called(ctx, id, deletedBy).Error(0)
}

func (m *MockMachineRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockMachineRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

// newMachineMB builds the "MB" machine row every create is expected to resolve.
func newMachineMB(t *testing.T) *machine.Entity {
	t.Helper()
	e, err := machine.New(
		mbhead.DefaultMachineCode, "Melange Batch", "MB", "PLANT",
		1, 1, 1, nil, 1, nil,
		nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		"", "admin",
	)
	require.NoError(t, err)
	return e
}

// newMBMachineRepo returns a machine repo that always resolves the "MB" machine.
// Shared with handlers_test.go, whose pre-existing create cases do not care which
// machine comes back — only that the lookup succeeds.
func newMBMachineRepo() *MockMachineRepository {
	m := new(MockMachineRepository)
	e, _ := machine.New(
		mbhead.DefaultMachineCode, "Melange Batch", "MB", "PLANT",
		1, 1, 1, nil, 1, nil,
		nil, nil, nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		"", "admin",
	)
	m.On("GetByCode", mock.Anything, mbhead.DefaultMachineCode).Return(e, nil).Maybe()
	return m
}

func strptr(s string) *string { return &s }

// --- K3: B5 machine is derived, never taken from the client -------------------------

func TestCreateHandler_MachineIsAlwaysMB(t *testing.T) {
	ctx := context.Background()
	mb := newMachineMB(t)

	t.Run("no machine_id supplied - head gets the MB machine", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin",
		})

		require.NoError(t, err)
		require.NotNil(t, got.MachineID())
		assert.Equal(t, mb.ID(), *got.MachineID())
		mrepo.AssertExpectations(t)
	})

	t.Run("a DIFFERENT machine_id supplied - still the MB machine", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, "MB002").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		other := uuid.New()
		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB002", CreatedBy: "admin", MachineID: &other,
		})

		require.NoError(t, err)
		require.NotNil(t, got.MachineID())
		assert.Equal(t, mb.ID(), *got.MachineID(), "client-supplied machine_id must be ignored")
		assert.NotEqual(t, other, *got.MachineID())
	})
}

// TestCreateHandler_MissingMBMachineIsAnError covers K3b: the head must NOT be
// created with a silently NULL machine when the MB machine row is absent.
func TestCreateHandler_MissingMBMachineIsAnError(t *testing.T) {
	ctx := context.Background()

	for name, lookupErr := range map[string]error{
		"machine not found": machine.ErrNotFound,
		"lookup fails":      errors.New("db down"),
	} {
		t.Run(name, func(t *testing.T) {
			repo := new(MockRepository)
			mrepo := new(MockMachineRepository)
			mrepo.On("GetByCode", ctx, "MB").Return(nil, lookupErr)
			repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

			got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
				MBCosting: "MB001", CreatedBy: "admin",
			})

			require.Error(t, err)
			assert.Nil(t, got)
			repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		})
	}

	t.Run("machine.ErrNotFound maps to ErrDefaultMachineUnavailable", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(nil, machine.ErrNotFound)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		_, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin",
		})
		assert.ErrorIs(t, err, mbhead.ErrDefaultMachineUnavailable)
	})
}

// --- K4: B7 default status ----------------------------------------------------------

func TestCreateHandler_DefaultStatus(t *testing.T) {
	ctx := context.Background()
	mb := newMachineMB(t)

	newHandler := func(costing string) (*mbhead.CreateHandler, *MockRepository) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, costing).Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		return mbhead.NewCreateHandler(repo, mrepo), repo
	}

	t.Run("status omitted - defaults to \"R and D\"", func(t *testing.T) {
		h, _ := newHandler("MB001")
		got, err := h.Handle(ctx, mbhead.CreateCommand{MBCosting: "MB001", CreatedBy: "admin"})
		require.NoError(t, err)
		require.NotNil(t, got.MBHStatus())
		assert.Equal(t, "R and D", *got.MBHStatus())
	})

	t.Run("status supplied - kept verbatim", func(t *testing.T) {
		h, _ := newHandler("MB001")
		got, err := h.Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", MBHStatus: strptr("Commercial"),
		})
		require.NoError(t, err)
		require.NotNil(t, got.MBHStatus())
		assert.Equal(t, "Commercial", *got.MBHStatus())
	})
}

// --- K2: a legacy payload must succeed and store NULLs, not defaults ----------------

func TestCreateHandler_LegacyPayloadStoresNulls(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	mrepo := new(MockMachineRepository)
	mrepo.On("GetByCode", ctx, "MB").Return(newMachineMB(t), nil)
	repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

	// Exactly the shape the legacy form sends: no mbhVsNumber, no mbhNoOfProcess,
	// no additionalShades.
	got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
		MBCosting: "MB001", CreatedBy: "admin",
	})

	require.NoError(t, err)
	assert.Nil(t, got.VSNumber(), "absent vs_number must stay NULL, not \"\"")
	assert.Nil(t, got.NoOfProcess(), "absent no_of_process must stay NULL — ⛔ NOT defaulted to \"D\" (U-B open)")
	assert.Nil(t, got.AdditionalShades())
	// K-1/U-E: the Oracle check-status column is never written from this path.
	assert.Nil(t, got.MBHCheckStatus())
	// No uniqueness probe happens when there is no VS Number to check.
	repo.AssertNotCalled(t, "ExistsByVSNumber", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestCreateHandler_StoresSuppliedRecipeFields is the counterpart: what IS sent is
// stored verbatim.
func TestCreateHandler_StoresSuppliedRecipeFields(t *testing.T) {
	ctx := context.Background()
	repo := new(MockRepository)
	mrepo := new(MockMachineRepository)
	mrepo.On("GetByCode", ctx, "MB").Return(newMachineMB(t), nil)
	repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
	repo.On("ExistsByVSNumber", ctx, "VS-9", uuid.Nil).Return(false, nil)
	repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

	got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
		MBCosting: "MB001", CreatedBy: "admin",
		VSNumber: strptr("VS-9"), NoOfProcess: strptr("T"),
	})

	require.NoError(t, err)
	require.NotNil(t, got.VSNumber())
	assert.Equal(t, "VS-9", *got.VSNumber())
	require.NotNil(t, got.NoOfProcess())
	assert.Equal(t, "T", *got.NoOfProcess())
}

// --- K5: B9 VS Number uniqueness ----------------------------------------------------

func TestCreateHandler_VSNumberUniqueness(t *testing.T) {
	ctx := context.Background()
	mb := newMachineMB(t)

	t.Run("duplicate on new data - ErrDuplicateVSNumber", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil).Maybe()
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("ExistsByVSNumber", ctx, "VS-1", uuid.Nil).Return(true, nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", VSNumber: strptr("VS-1"),
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateVSNumber)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	// The 177 production heads carrying '0' must remain creatable/savable, so '0'
	// (and "") are never probed at all.
	for _, exempt := range []string{"0", ""} {
		t.Run("exempt value "+`"`+exempt+`"`+" is never checked", func(t *testing.T) {
			repo := new(MockRepository)
			mrepo := new(MockMachineRepository)
			mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
			repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
			repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

			got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
				MBCosting: "MB001", CreatedBy: "admin", VSNumber: strptr(exempt),
			})

			require.NoError(t, err)
			require.NotNil(t, got.VSNumber())
			assert.Equal(t, exempt, *got.VSNumber())
			repo.AssertNotCalled(t, "ExistsByVSNumber", mock.Anything, mock.Anything, mock.Anything)
		})
	}

	// J5: "NA" is a literal, legitimate VS Number. No regex, no normalization.
	t.Run("literal \"NA\" passes through", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("ExistsByVSNumber", ctx, "NA", uuid.Nil).Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", VSNumber: strptr("NA"),
		})

		require.NoError(t, err)
		require.NotNil(t, got.VSNumber())
		assert.Equal(t, "NA", *got.VSNumber())
	})
}

func TestUpdateHandler_VSNumberOnlyCheckedWhenChanged(t *testing.T) {
	ctx := context.Background()

	// storedWithVS builds a persisted head already carrying a VS Number.
	storedWithVS := func(t *testing.T, vs string) *mbheaddomain.Entity {
		t.Helper()
		e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "MB001", CreatedBy: "admin"})
		require.NoError(t, err)
		e.HydrateExtras(mbheaddomain.PersistedExtras{VSNumber: &vs})
		return e
	}

	t.Run("unchanged value is not probed", func(t *testing.T) {
		e := storedWithVS(t, "VS-7")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", VSNumber: strptr("VS-7"),
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByVSNumber", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("changed value is probed and a duplicate is rejected", func(t *testing.T) {
		e := storedWithVS(t, "VS-7")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("ExistsByVSNumber", ctx, "VS-8", e.ID()).Return(true, nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", VSNumber: strptr("VS-8"),
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateVSNumber)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	// The legacy '0' block: re-saving a head whose VS Number is '0' must never probe.
	t.Run("legacy '0' head re-saved with '0' is not probed", func(t *testing.T) {
		e := storedWithVS(t, "0")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", VSNumber: strptr("0"),
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByVSNumber", mock.Anything, mock.Anything, mock.Anything)
	})
}

// --- K6: a legacy head with a NULL cross_section must stay updatable ---------------

func TestUpdateHandler_LegacyHeadWithNullCrossSectionSucceeds(t *testing.T) {
	ctx := context.Background()

	// Reconstruct mimics a real legacy row: cross_section empty (NULL in storage),
	// no mgt_name, no denier — the shape of the 573 heads in production.
	e := mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB-LEGACY", nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, true, time.Now(), "oracle",
		nil, nil, nil, nil,
		"DRAFT", false, 0, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "", "",
		nil,
	)
	require.Empty(t, e.CrossSection())

	repo := new(MockRepository)
	repo.On("GetByID", ctx, e.ID()).Return(e, nil)
	repo.On("Update", ctx, e).Return(nil)

	got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
		ID: e.ID(), UpdatedBy: "admin", MgtName: strptr("renamed"),
	})

	require.NoError(t, err, "a legacy head with NULL cross_section must remain editable")
	require.NotNil(t, got.MgtName())
	assert.Equal(t, "renamed", *got.MgtName())
	assert.Empty(t, got.CrossSection())
}

// --- Shades -------------------------------------------------------------------------

// TestUpdateHandler_ShadesUntouchedWithoutFlag is the guard against the legacy edit
// form silently wiping every shade row on each save.
func TestUpdateHandler_ShadesUntouchedWithoutFlag(t *testing.T) {
	ctx := context.Background()

	stored := func(t *testing.T) *mbheaddomain.Entity {
		t.Helper()
		e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: "MB001", CreatedBy: "admin"})
		require.NoError(t, err)
		e.HydrateExtras(mbheaddomain.PersistedExtras{
			AdditionalShades: []mbheaddomain.Shade{{SeqNo: 1, Code: "SH-A", Name: "Alpha"}},
		})
		return e
	}

	t.Run("flag absent - stored shades survive, no write", func(t *testing.T) {
		e := stored(t)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin",
		})

		require.NoError(t, err)
		require.Len(t, got.AdditionalShades(), 1)
		assert.Equal(t, "SH-A", got.AdditionalShades()[0].Code)
		repo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	// Even a payload that carries shades must not touch them without the flag.
	t.Run("shades supplied but flag false - still untouched", func(t *testing.T) {
		e := stored(t)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin",
			AdditionalShades:        []mbheaddomain.Shade{{SeqNo: 1, Code: "SH-Z"}},
			ReplaceAdditionalShades: false,
		})

		require.NoError(t, err)
		require.Len(t, got.AdditionalShades(), 1)
		assert.Equal(t, "SH-A", got.AdditionalShades()[0].Code)
		repo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("flag true - shades replaced", func(t *testing.T) {
		e := stored(t)
		repo := new(MockRepository)
		want := []mbheaddomain.Shade{{SeqNo: 1, Code: "SH-Z", Name: ""}}
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)
		repo.On("ReplaceShades", ctx, e.ID(), want, "admin").Return(nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin",
			AdditionalShades: want, ReplaceAdditionalShades: true,
		})

		require.NoError(t, err)
		require.Len(t, got.AdditionalShades(), 1)
		assert.Equal(t, "SH-Z", got.AdditionalShades()[0].Code)
		repo.AssertExpectations(t)
	})

	t.Run("flag true with empty list - shades cleared", func(t *testing.T) {
		e := stored(t)
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)
		repo.On("ReplaceShades", ctx, e.ID(), []mbheaddomain.Shade(nil), "admin").Return(nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", ReplaceAdditionalShades: true,
		})

		require.NoError(t, err)
		assert.Empty(t, got.AdditionalShades())
		repo.AssertExpectations(t)
	})
}

// TestCreateHandler_ShadesAllowedAtCreate covers U-C (2026-08-22).
func TestCreateHandler_ShadesAllowedAtCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("valid shades are written", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(newMachineMB(t), nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
		// Name may be empty — mbhs_shade_name is nullable.
		want := []mbheaddomain.Shade{{SeqNo: 1, Code: "SH-A", Name: "Alpha"}, {SeqNo: 2, Code: "SH-B"}}
		repo.On("ReplaceShades", ctx, mock.AnythingOfType("uuid.UUID"), want, "admin").Return(nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", AdditionalShades: want,
		})

		require.NoError(t, err)
		assert.Len(t, got.AdditionalShades(), 2)
		repo.AssertExpectations(t)
	})

	t.Run("three shades are rejected before the head row is written", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(newMachineMB(t), nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin",
			AdditionalShades: []mbheaddomain.Shade{
				{SeqNo: 1, Code: "A"}, {SeqNo: 2, Code: "B"}, {SeqNo: 3, Code: "C"},
			},
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrTooManyShades)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		repo.AssertNotCalled(t, "ReplaceShades", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// --- U-D: Dev Code uniqueness (2026-08-22) ------------------------------------------
//
// The rule the user stated: "dev code harus unique, tetapi jika ada data legacy yg
// sama biarkan saja" — unique for NEW data, existing legacy duplicates untouched.
// That is the same shape as B9 (VS Number), so these tests mirror the VS Number
// tests above rather than inventing a second pattern.

func TestCreateHandler_DevCodeUniqueness(t *testing.T) {
	ctx := context.Background()
	mb := newMachineMB(t)

	t.Run("duplicate on new data - ErrDuplicateDevCode", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil).Maybe()
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("ExistsByDevCode", ctx, "DEV-1", uuid.Nil).Return(true, nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", DevCode: "DEV-1",
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateDevCode)
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("free dev code is created", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("ExistsByDevCode", ctx, "DEV-2", uuid.Nil).Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		got, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin", DevCode: "DEV-2",
		})

		require.NoError(t, err)
		assert.Equal(t, "DEV-2", got.DevCode())
	})

	// An absent Dev Code must never be probed: "" is the legacy shape and probing it
	// would make every head without a Dev Code collide with the next one.
	t.Run("empty dev code is never checked", func(t *testing.T) {
		repo := new(MockRepository)
		mrepo := new(MockMachineRepository)
		mrepo.On("GetByCode", ctx, "MB").Return(mb, nil)
		repo.On("ExistsByMBCosting", ctx, "MB001").Return(false, nil)
		repo.On("Create", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)

		_, err := mbhead.NewCreateHandler(repo, mrepo).Handle(ctx, mbhead.CreateCommand{
			MBCosting: "MB001", CreatedBy: "admin",
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByDevCode", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestUpdateHandler_DevCodeOnlyCheckedWhenChanged(t *testing.T) {
	ctx := context.Background()

	// storedWithDev builds a persisted head already carrying a Dev Code.
	storedWithDev := func(t *testing.T, dev string) *mbheaddomain.Entity {
		t.Helper()
		e, err := mbheaddomain.New(mbheaddomain.NewParams{
			MBCosting: "MB001", CreatedBy: "admin", DevCode: dev,
		})
		require.NoError(t, err)
		return e
	}

	// This is the legacy-tolerance case: two production heads may already share
	// 'DEV-LEGACY'. Re-saving one of them without touching the Dev Code must not
	// probe, and therefore must not fail.
	t.Run("unchanged value is not probed - legacy duplicates stay editable", func(t *testing.T) {
		e := storedWithDev(t, "DEV-LEGACY")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", DevCode: strptr("DEV-LEGACY"),
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByDevCode", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("absent dev code is not probed", func(t *testing.T) {
		e := storedWithDev(t, "DEV-LEGACY")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin",
		})

		require.NoError(t, err)
		repo.AssertNotCalled(t, "ExistsByDevCode", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("changed value is probed and a duplicate is rejected", func(t *testing.T) {
		e := storedWithDev(t, "DEV-7")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("ExistsByDevCode", ctx, "DEV-8", e.ID()).Return(true, nil)

		got, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", DevCode: strptr("DEV-8"),
		})

		assert.Nil(t, got)
		assert.ErrorIs(t, err, mbheaddomain.ErrDuplicateDevCode)
		repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	})

	t.Run("changed value that is free is accepted", func(t *testing.T) {
		e := storedWithDev(t, "DEV-7")
		repo := new(MockRepository)
		repo.On("GetByID", ctx, e.ID()).Return(e, nil)
		repo.On("ExistsByDevCode", ctx, "DEV-9", e.ID()).Return(false, nil)
		repo.On("Update", ctx, e).Return(nil)

		_, err := mbhead.NewUpdateHandler(repo).Handle(ctx, mbhead.UpdateCommand{
			ID: e.ID(), UpdatedBy: "admin", DevCode: strptr("DEV-9"),
		})

		require.NoError(t, err)
	})
}
