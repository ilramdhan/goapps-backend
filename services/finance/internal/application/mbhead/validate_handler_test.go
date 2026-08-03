package mbhead_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	mbparamdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// MockParamRepository is a mock implementation of mbparam.Repository. Only ListActive
// is exercised by ValidateHandler; the rest satisfy the interface.
type MockParamRepository struct {
	mock.Mock
}

func (m *MockParamRepository) Create(ctx context.Context, e *mbparamdomain.Entity) error {
	return m.Called(ctx, e).Error(0)
}

func (m *MockParamRepository) Update(ctx context.Context, e *mbparamdomain.Entity) error {
	return m.Called(ctx, e).Error(0)
}

func (m *MockParamRepository) Delete(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockParamRepository) GetByID(ctx context.Context, id string) (*mbparamdomain.Entity, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbparamdomain.Entity), args.Error(1)
}

func (m *MockParamRepository) List(
	ctx context.Context, filter mbparamdomain.ListFilter,
) ([]*mbparamdomain.Entity, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*mbparamdomain.Entity), args.Get(1).(int64), args.Error(2)
}

func (m *MockParamRepository) ListActive(ctx context.Context) ([]*mbparamdomain.Entity, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*mbparamdomain.Entity), args.Error(1)
}

func (m *MockParamRepository) ListAll(
	ctx context.Context, filter mbparamdomain.ExportFilter,
) ([]*mbparamdomain.Entity, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*mbparamdomain.Entity), args.Error(1)
}

func (m *MockParamRepository) GetByCode(ctx context.Context, code string) (*mbparamdomain.Entity, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*mbparamdomain.Entity), args.Error(1)
}

func (m *MockParamRepository) CreateOption(ctx context.Context, o *mbparamdomain.Option) error {
	return m.Called(ctx, o).Error(0)
}

func (m *MockParamRepository) UpdateOption(ctx context.Context, o *mbparamdomain.Option) error {
	return m.Called(ctx, o).Error(0)
}

func (m *MockParamRepository) DeleteOption(ctx context.Context, id string) error {
	return m.Called(ctx, id).Error(0)
}

// activeParamMasters returns the mst_mb_param default set as production has it:
// throughput default option 'B' (=40) and no_of_process default option 'D' (=2).
// These are the values that were incorrectly frozen onto every head.
func activeParamMasters(t *testing.T) []*mbparamdomain.Entity {
	t.Helper()
	specs := []struct {
		code, paramType, defaultValue, defaultOption string
	}{
		{"WASTE", mbparamdomain.TypeScalar, "2", ""},
		{"QUALITY_LOSS", mbparamdomain.TypeScalar, "0.6", ""},
		{"EFFICIENCY", mbparamdomain.TypeScalar, "94", ""},
		{"DEV_EXPENSE", mbparamdomain.TypeScalar, "1", ""},
		{"PACKING", mbparamdomain.TypeScalar, "3", ""},
		{"MB_PROD_PER_DAY", mbparamdomain.TypeScalar, "16", ""},
		{"THROUGHPUT_PER_HOUR", mbparamdomain.TypePicklist, "", "B"},
		{"NO_OF_PROCESS", mbparamdomain.TypePicklist, "", "D"},
	}
	out := make([]*mbparamdomain.Entity, 0, len(specs))
	for i, s := range specs {
		e, err := mbparamdomain.NewEntity(
			s.code, s.code, s.paramType, "", s.defaultValue, s.defaultOption, "", int32(i), "seed",
		)
		require.NoError(t, err)
		out = append(out, e)
	}
	return out
}

// approvedHeadWithParams builds an APPROVED own-production head carrying its own
// per-product throughput / no_of_process / prod_per_day values (as DATA-MB-02 set them).
func approvedHeadWithParams(throughput, noOfProcess string, prodPerDay *string) *mbheaddomain.Entity {
	return mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbheaddomain.StatusApproved, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		prodPerDay, throughput, noOfProcess,
		nil,
	)
}

func strPtr(s string) *string { return &s }

// TestValidateHandler_Handle_PrefersHeadThroughputOverMasterDefault is the core
// ENG-MB-01 regression test. Before the fix, freeze always took the param-master
// picklist default ('B'=40, 'D'=2), so MB_NET_PROD was 40*0.94*16=601.6 for every
// MB and conversion cost came out flat (~3.4) regardless of product. The head's own
// values must win.
func TestValidateHandler_Handle_PrefersHeadThroughputOverMasterDefault(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewValidateHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", strPtr("18"))

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var captured *mbheaddomain.ParamSnapshot
	mockRepo.On("TransitionWithAutoGen",
		ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		int32(2), "", "tester",
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
		mock.AnythingOfType("*mbhead.Entity"),
	).Run(func(args mock.Arguments) {
		captured = args.Get(7).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	result, err := handler.Handle(ctx, mbhead.ValidateCommand{
		MbhID: entity.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// The snapshot persisted to mst_mb_head + frozen onto CPP must carry the head's values.
	require.NotNil(t, captured)
	assert.Equal(t, "34", captured.ThroughputPerHour, "throughput must come from the head, not the 'B' default")
	assert.Equal(t, "S", captured.NoOfProcess, "no_of_process must come from the head, not the 'D' default")
	require.NotNil(t, captured.MBProdPerDay)
	assert.Equal(t, "18", *captured.MBProdPerDay, "mb_prod_per_day must come from the head when set")

	// The in-memory entity must agree — mbFreezeCostParams reads these getters
	// when writing cost_product_parameter.
	assert.Equal(t, "34", result.ParamThroughputPerHour())
	assert.Equal(t, "S", result.ParamNoOfProcess())

	// Scalar params legitimately stay on the master defaults.
	require.NotNil(t, captured.Efficiency)
	assert.Equal(t, "94", *captured.Efficiency)

	mockRepo.AssertExpectations(t)
	mockParams.AssertExpectations(t)
}

// TestValidateHandler_Handle_FallsBackToMasterDefaultsWhenHeadEmpty ensures heads
// that never had per-product values set still freeze successfully on the master
// defaults, rather than freezing empty strings (which would break the
// mst_mb_param_option numeric lookup during auto-gen).
func TestValidateHandler_Handle_FallsBackToMasterDefaultsWhenHeadEmpty(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewValidateHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("", "", nil)

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var captured *mbheaddomain.ParamSnapshot
	mockRepo.On("TransitionWithAutoGen",
		ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		int32(2), "", "tester",
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
		mock.AnythingOfType("*mbhead.Entity"),
	).Run(func(args mock.Arguments) {
		captured = args.Get(7).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	_, err := handler.Handle(ctx, mbhead.ValidateCommand{
		MbhID: entity.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, "B", captured.ThroughputPerHour, "empty head value must fall back to the master default")
	assert.Equal(t, "D", captured.NoOfProcess, "empty head value must fall back to the master default")
	require.NotNil(t, captured.MBProdPerDay)
	assert.Equal(t, "16", *captured.MBProdPerDay, "nil head value must fall back to the master default")

	mockRepo.AssertExpectations(t)
	mockParams.AssertExpectations(t)
}

// TestValidateHandler_Handle_HeadWithEmptyProdPerDayPointerFallsBack covers the
// pointer-to-empty-string case: mbh_param_mb_prod_per_day exists but holds "".
// Freezing "" would make NULLIF(...,”)::numeric write NULL and break the calc.
func TestValidateHandler_Handle_HeadWithEmptyProdPerDayPointerFallsBack(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewValidateHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("55", "T", strPtr(""))

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var captured *mbheaddomain.ParamSnapshot
	mockRepo.On("TransitionWithAutoGen",
		ctx, entity.ID(), mbheaddomain.StatusApproved, mbheaddomain.StatusValidated,
		int32(2), "", "tester",
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
		mock.AnythingOfType("*mbhead.Entity"),
	).Run(func(args mock.Arguments) {
		captured = args.Get(7).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	_, err := handler.Handle(ctx, mbhead.ValidateCommand{
		MbhID: entity.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, "55", captured.ThroughputPerHour)
	require.NotNil(t, captured.MBProdPerDay)
	assert.Equal(t, "16", *captured.MBProdPerDay, "pointer-to-empty must fall back, not freeze an empty string")

	mockRepo.AssertExpectations(t)
	mockParams.AssertExpectations(t)
}

// TestValidateHandler_Handle_BoughtoutFromDraftUsesHeadParams verifies the boughtout
// shortcut (DRAFT -> VALIDATED) applies the same head-preference rule.
func TestValidateHandler_Handle_BoughtoutFromDraftUsesHeadParams(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewValidateHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB-BOUGHT", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbheaddomain.StatusDraft, true, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "20", "S",
		nil,
	)

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var captured *mbheaddomain.ParamSnapshot
	mockRepo.On("TransitionWithAutoGen",
		ctx, entity.ID(), mbheaddomain.StatusDraft, mbheaddomain.StatusValidated,
		int32(2), "", "tester",
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
		mock.AnythingOfType("*mbhead.Entity"),
	).Run(func(args mock.Arguments) {
		captured = args.Get(7).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	_, err := handler.Handle(ctx, mbhead.ValidateCommand{
		MbhID: entity.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, "20", captured.ThroughputPerHour)
	assert.Equal(t, "S", captured.NoOfProcess)

	mockRepo.AssertExpectations(t)
	mockParams.AssertExpectations(t)
}

// TestValidateHandler_Handle_RejectsNonApprovedOwnProduction pins the pre-existing
// gate so the ENG-MB-01 change does not loosen it.
func TestValidateHandler_Handle_RejectsNonApprovedOwnProduction(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewValidateHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", nil)
	draft := mbheaddomain.Reconstruct(
		entity.ID(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbheaddomain.StatusDraft, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "34", "S",
		nil,
	)

	mockRepo.On("GetByID", ctx, draft.ID()).Return(draft, nil)

	result, err := handler.Handle(ctx, mbhead.ValidateCommand{
		MbhID: draft.ID(), ActorUserID: "tester",
	})

	assert.Nil(t, result)
	assert.ErrorIs(t, err, mbheaddomain.ErrInvalidTransition)
	mockParams.AssertNotCalled(t, "ListActive", ctx)
	mockRepo.AssertExpectations(t)
}
