package rmgroup_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

func newHead(t *testing.T) *rmgroup.Head {
	t.Helper()
	code, err := rmgroup.NewCode("GRP-TEST")
	require.NoError(t, err)
	head, err := rmgroup.NewHead(code, "Test Group", "", 1.0, 10.0, "user:test")
	require.NoError(t, err)
	return head
}

func TestCreateHandler_Success(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)
	repo.On("ExistsHeadByCode", ctx, mock.AnythingOfType("rmgroup.Code")).Return(false, nil)
	repo.On("CreateHead", ctx, mock.AnythingOfType("*rmgroup.Head")).Return(nil)

	h := appgroup.NewCreateHandler(repo)
	out, err := h.Handle(ctx, appgroup.CreateCommand{
		Code:           "PIG-001",
		Name:           "Pigment One",
		CostPercentage: 1.1,
		CostPerKg:      5.0,
		CreatedBy:      "user:test",
	})
	require.NoError(t, err)
	assert.Equal(t, "PIG-001", out.Code().String())
	repo.AssertExpectations(t)
}

func TestCreateHandler_DuplicateCode(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)
	repo.On("ExistsHeadByCode", ctx, mock.AnythingOfType("rmgroup.Code")).Return(true, nil)

	h := appgroup.NewCreateHandler(repo)
	_, err := h.Handle(ctx, appgroup.CreateCommand{
		Code: "PIG-001", Name: "X", CostPercentage: 1, CostPerKg: 1, CreatedBy: "u",
	})
	assert.ErrorIs(t, err, rmgroup.ErrCodeAlreadyExists)
}

func TestCreateHandler_InvalidCode(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)

	h := appgroup.NewCreateHandler(repo)
	_, err := h.Handle(ctx, appgroup.CreateCommand{Code: "", Name: "X", CreatedBy: "u"})
	assert.ErrorIs(t, err, rmgroup.ErrEmptyCode)
	repo.AssertNotCalled(t, "ExistsHeadByCode")
}

func TestGetHandler_InvalidID(t *testing.T) {
	h := appgroup.NewGetHandler(new(mockRepo))
	_, err := h.Handle(context.Background(), appgroup.GetQuery{HeadID: "not-a-uuid"})
	assert.ErrorIs(t, err, rmgroup.ErrNotFound)
}

func TestGetHandler_WithDetails(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("ListActiveDetailsByHeadID", ctx, head.ID()).Return([]*rmgroup.Detail{}, nil)

	h := appgroup.NewGetHandler(repo)
	res, err := h.Handle(ctx, appgroup.GetQuery{HeadID: head.ID().String(), WithDetails: true, ActiveOnly: true})
	require.NoError(t, err)
	assert.Equal(t, head, res.Head)
	repo.AssertExpectations(t)
}

func TestListHandler_Pagination(t *testing.T) {
	ctx := context.Background()
	repo := new(mockRepo)
	repo.On("ListHeads", ctx, mock.AnythingOfType("rmgroup.ListFilter")).
		Return([]*rmgroup.Head{newHead(t)}, int64(25), nil)

	h := appgroup.NewListHandler(repo)
	res, err := h.Handle(ctx, appgroup.ListQuery{Page: 2, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(25), res.TotalItems)
	assert.Equal(t, int32(3), res.TotalPages)
	assert.Equal(t, int32(2), res.CurrentPage)
}

func TestListHandler_InvalidFlag(t *testing.T) {
	repo := new(mockRepo)
	h := appgroup.NewListHandler(repo)
	_, err := h.Handle(context.Background(), appgroup.ListQuery{Flag: "BOGUS"})
	assert.ErrorIs(t, err, rmgroup.ErrInvalidFlag)
}

func TestUpdateHandler_Success(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("UpdateHead", ctx, head).Return(nil)

	newName := "Updated Name"
	h := appgroup.NewUpdateHandler(repo)
	out, err := h.Handle(ctx, appgroup.UpdateCommand{
		HeadID:    head.ID().String(),
		Name:      &newName,
		UpdatedBy: "user:edit",
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", out.Name())
}

func TestUpdateHandler_InvalidFlag(t *testing.T) {
	head := newHead(t)
	repo := new(mockRepo)
	repo.On("GetHeadByID", mock.Anything, head.ID()).Return(head, nil)

	bad := "BOGUS"
	h := appgroup.NewUpdateHandler(repo)
	_, err := h.Handle(context.Background(), appgroup.UpdateCommand{
		HeadID:        head.ID().String(),
		FlagValuation: &bad,
		UpdatedBy:     "user:edit",
	})
	assert.ErrorIs(t, err, rmgroup.ErrInvalidFlag)
}

func TestDeleteHandler_Success(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := new(mockRepo)
	repo.On("ExistsHeadByID", ctx, id).Return(true, nil)
	repo.On("SoftDeleteHead", ctx, id, "user:del").Return(nil)

	h := appgroup.NewDeleteHandler(repo, nil)
	err := h.Handle(ctx, appgroup.DeleteCommand{HeadID: id.String(), DeletedBy: "user:del"})
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestDeleteHandler_NotFound(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repo := new(mockRepo)
	repo.On("ExistsHeadByID", ctx, id).Return(false, nil)

	h := appgroup.NewDeleteHandler(repo, nil)
	err := h.Handle(ctx, appgroup.DeleteCommand{HeadID: id.String(), DeletedBy: "u"})
	assert.ErrorIs(t, err, rmgroup.ErrNotFound)
}

func TestAddItemsHandler_SkipsItemInAnotherGroup(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)

	// Simulate item ABC-123 already held by a DIFFERENT group.
	otherCode, _ := rmgroup.NewItemCode("ABC-123")
	otherDetail, err := rmgroup.NewDetail(uuid.New(), otherCode, "user:old")
	require.NoError(t, err)
	repo.On("GetActiveDetailByItemCodeGrade", ctx, mock.AnythingOfType("rmgroup.ItemCode"), mock.AnythingOfType("string")).
		Return(otherDetail, nil).Once()

	// A second item is free; should be created.
	freeCode, _ := rmgroup.NewItemCode("FREE-9")
	_ = freeCode
	repo.On("GetActiveDetailByItemCodeGrade", ctx, mock.AnythingOfType("rmgroup.ItemCode"), mock.AnythingOfType("string")).
		Return(nil, rmgroup.ErrDetailNotFound).Once()
	repo.On("AddDetail", ctx, mock.AnythingOfType("*rmgroup.Detail")).Return(nil).Once()

	h := appgroup.NewAddItemsHandler(repo)
	res, err := h.Handle(ctx, appgroup.AddItemsCommand{
		HeadID:    head.ID().String(),
		CreatedBy: "user:new",
		Items: []appgroup.AddItemInput{
			{ItemCode: "ABC-123"},
			{ItemCode: "FREE-9"},
		},
	})
	require.NoError(t, err)
	assert.Len(t, res.Added, 1)
	assert.Len(t, res.Skipped, 1)
	assert.Equal(t, "ABC-123", res.Skipped[0].ItemCode)
}

func TestRemoveItemsHandler_SoftDelete(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	itemCode, _ := rmgroup.NewItemCode("X-1")
	detail, err := rmgroup.NewDetail(head.ID(), itemCode, "user:test")
	require.NoError(t, err)

	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetDetailByID", ctx, detail.ID()).Return(detail, nil)
	repo.On("SoftDeleteDetail", ctx, detail.ID(), "user:del").Return(nil)

	h := appgroup.NewRemoveItemsHandler(repo)
	res, err := h.Handle(ctx, appgroup.RemoveItemsCommand{
		HeadID:    head.ID().String(),
		DetailIDs: []string{detail.ID().String()},
		Mode:      appgroup.RemoveModeSoftDelete,
		RemovedBy: "user:del",
	})
	require.NoError(t, err)
	assert.Len(t, res.Removed, 1)
}

func TestRemoveItemsHandler_RejectsMismatchedHead(t *testing.T) {
	ctx := context.Background()
	head := newHead(t)
	otherHeadID := uuid.New()
	itemCode, _ := rmgroup.NewItemCode("Y-1")
	// Detail belongs to otherHeadID, not head.
	detail, err := rmgroup.NewDetail(otherHeadID, itemCode, "user:x")
	require.NoError(t, err)

	repo := new(mockRepo)
	repo.On("GetHeadByID", ctx, head.ID()).Return(head, nil)
	repo.On("GetDetailByID", ctx, detail.ID()).Return(detail, nil)

	h := appgroup.NewRemoveItemsHandler(repo)
	_, err = h.Handle(ctx, appgroup.RemoveItemsCommand{
		HeadID:    head.ID().String(),
		DetailIDs: []string{detail.ID().String()},
		RemovedBy: "user:del",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong")
}

func TestUngroupedHandler_Pagination(t *testing.T) {
	ctx := context.Background()
	reader := new(mockUngroupedReader)
	reader.On("ListUngroupedItems", ctx, mock.AnythingOfType("rmgroup.UngroupedItemsFilter")).
		Return([]*syncdata.ItemConsStockPO{{ItemCode: "A"}}, int64(7), nil)

	h := appgroup.NewUngroupedHandler(reader)
	res, err := h.Handle(ctx, appgroup.UngroupedQuery{Page: 1, PageSize: 5, Period: "202604"})
	require.NoError(t, err)
	assert.Equal(t, int64(7), res.TotalItems)
	assert.Equal(t, int32(2), res.TotalPages)
	assert.Len(t, res.Items, 1)
}
