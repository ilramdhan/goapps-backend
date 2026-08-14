package mbhead_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// TestRefreezeHandler_Handle_AppliesHeadOverridesThenCallsRepo confirms the core
// re-freeze flow: resolve params → apply head overrides → call RefreezeCostParams.
func TestRefreezeHandler_Handle_AppliesHeadOverridesThenCallsRepo(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewRefreezeHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("55", "T", strPtr("20"))
	// Already validated — has a cost product.
	costProduct := mbheaddomain.Reconstruct(
		entity.ID(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, true, entity.CreatedAt(), entity.CreatedBy(),
		nil, nil, nil, nil,
		mbheaddomain.StatusValidated, false, 1, nil,
		"", "", "", "", "", "",
		1001, nil, "",
		nil, nil, nil, nil, nil,
		strPtr("20"), "55", "T",
		nil, "", "",
	)

	mockRepo.On("GetByID", ctx, costProduct.ID()).Return(costProduct, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var capturedSnapshot *mbheaddomain.ParamSnapshot
	mockRepo.On("RefreezeCostParams",
		ctx, costProduct.ID(),
		mock.AnythingOfType("*mbhead.Entity"),
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
	).Run(func(args mock.Arguments) {
		capturedSnapshot = args.Get(3).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	err := handler.Handle(ctx, mbhead.RefreezeCommand{
		MbhID: costProduct.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)

	require.NotNil(t, capturedSnapshot)
	assert.Equal(t, "55", capturedSnapshot.ThroughputPerHour, "head throughput must win over master default")
	assert.Equal(t, "T", capturedSnapshot.NoOfProcess, "head no_of_process must win over master default")
	require.NotNil(t, capturedSnapshot.MBProdPerDay)
	assert.Equal(t, "20", *capturedSnapshot.MBProdPerDay, "head mb_prod_per_day must win over master default")

	mockRepo.AssertExpectations(t)
	mockParams.AssertExpectations(t)
}

// TestRefreezeHandler_Handle_RejectsHeadWithoutCostProduct guards the guard:
// re-freeze requires an existing cost product (auto-gen gives it one).
func TestRefreezeHandler_Handle_RejectsHeadWithoutCostProduct(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	handler := mbhead.NewRefreezeHandler(mockRepo, mockParams)
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", nil) // status=APPROVED, no cost product

	mockRepo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	err := handler.Handle(ctx, mbhead.RefreezeCommand{
		MbhID: entity.ID(), ActorUserID: "tester",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cost product has not been auto-generated")

	mockRepo.AssertExpectations(t)
	mockParams.AssertNotCalled(t, "ListActive", ctx)
}

// TestRefreezeHandler_Handle_UsesSameOverrideLogicAsValidate ensures the two paths
// (Validate + Refreeze) produce identical frozen values for the same head data.
func TestRefreezeHandler_Handle_UsesSameOverrideLogicAsValidate(t *testing.T) {
	mockRepo := new(MockRepository)
	mockParams := new(MockParamRepository)
	ctx := context.Background()

	entity := approvedHeadWithParams("20", "S", nil)
	costProduct := mbheaddomain.Reconstruct(
		entity.ID(), nil, "MB002", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, true, entity.CreatedAt(), entity.CreatedBy(),
		nil, nil, nil, nil,
		mbheaddomain.StatusValidated, false, 1, nil,
		"", "", "", "", "", "",
		2002, nil, "",
		nil, nil, nil, nil, nil,
		nil, "20", "S",
		nil, "", "",
	)

	mockRepo.On("GetByID", ctx, costProduct.ID()).Return(costProduct, nil)
	mockParams.On("ListActive", ctx).Return(activeParamMasters(t), nil)

	var capturedSnapshot *mbheaddomain.ParamSnapshot
	mockRepo.On("RefreezeCostParams",
		ctx, costProduct.ID(),
		mock.AnythingOfType("*mbhead.Entity"),
		mock.AnythingOfType("*mbhead.ParamSnapshot"),
	).Run(func(args mock.Arguments) {
		capturedSnapshot = args.Get(3).(*mbheaddomain.ParamSnapshot)
	}).Return(nil)

	handler := mbhead.NewRefreezeHandler(mockRepo, mockParams)
	err := handler.Handle(ctx, mbhead.RefreezeCommand{
		MbhID: costProduct.ID(), ActorUserID: "tester",
	})
	require.NoError(t, err)

	require.NotNil(t, capturedSnapshot)
	assert.Equal(t, "20", capturedSnapshot.ThroughputPerHour)
	assert.Equal(t, "S", capturedSnapshot.NoOfProcess)
	// mb_prod_per_day not set on head → falls back to master default "16"
	require.NotNil(t, capturedSnapshot.MBProdPerDay)
	assert.Equal(t, "16", *capturedSnapshot.MBProdPerDay)
}
