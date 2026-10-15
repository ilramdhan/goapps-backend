package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
)

type fakeLSPlanner struct {
	items []lookupsync.Item
	sum   lookupsync.Summary
	err   error
	scope lookupsync.Scope
}

func (f *fakeLSPlanner) Walk(_ context.Context, scope lookupsync.Scope, visit func(lookupsync.Item) error) (lookupsync.Summary, error) {
	f.scope = scope
	if f.err != nil {
		return lookupsync.Summary{}, f.err
	}
	if visit != nil {
		for _, it := range f.items {
			if err := visit(it); err != nil {
				return lookupsync.Summary{}, err
			}
		}
	}
	return f.sum, nil
}

type fakeLSEnqueuer struct {
	called      int
	scope       lookupsync.Scope
	fingerprint string
}

func (f *fakeLSEnqueuer) EnqueueLookupSync(_ context.Context, s lookupsync.Scope, fp, _ string) (LookupSyncJobInfo, error) {
	f.called++
	f.scope, f.fingerprint = s, fp
	return LookupSyncJobInfo{JobID: "j1", JobCode: "JOB-1", Status: "QUEUED", TotalChildren: 3}, nil
}

func lsItems(n int) []lookupsync.Item {
	out := make([]lookupsync.Item, 0, n)
	for i := 0; i < n; i++ {
		it := lookupsync.Item{ProductSysID: int64(i + 1), ProductCode: "P"}
		if i%2 == 0 {
			it.Changes = []lookupsync.ValueChange{{ParamID: uuid.New(), ParamCode: "X", OldValue: "1", NewValue: "2", Kind: lookupsync.ChangeSet}}
		} else {
			it.SkipReason = lookupsync.SkipLocked
		}
		out = append(out, it)
	}
	return out
}

func masterScope() *financev1.LookupSyncScope {
	return &financev1.LookupSyncScope{Scope: &financev1.LookupSyncScope_MasterRef{
		MasterRef: &financev1.LookupMasterRef{LookupMasterCode: "MACHINE", MasterKey: "M1"},
	}}
}

func TestLookupSyncScopeFromProto(t *testing.T) {
	s, err := lookupSyncScopeFromProto(masterScope())
	require.NoError(t, err)
	assert.Equal(t, &lookupsync.MasterRef{LookupMasterCode: "MACHINE", MasterKey: "M1"}, s.MasterRef)

	s, err = lookupSyncScopeFromProto(&financev1.LookupSyncScope{Scope: &financev1.LookupSyncScope_ProductIds{
		ProductIds: &financev1.LookupSyncProductIDs{ProductSysIds: []int64{1, 2}}}})
	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2}, s.ProductSysIDs)

	s, err = lookupSyncScopeFromProto(&financev1.LookupSyncScope{Scope: &financev1.LookupSyncScope_Filter{
		Filter: &financev1.LookupSyncProductFilter{Search: "a", ProductTypeIds: []int32{3}, ShadeCode: "S", ActiveFilter: "active"}}})
	require.NoError(t, err)
	assert.Equal(t, &lookupsync.ProductFilter{Search: "a", ProductTypeIDs: []int32{3}, ShadeCode: "S", ActiveFilter: "active"}, s.Filter)

	_, err = lookupSyncScopeFromProto(nil)
	assert.Error(t, err)
	_, err = lookupSyncScopeFromProto(&financev1.LookupSyncScope{Scope: &financev1.LookupSyncScope_ProductIds{}})
	assert.Error(t, err)
}

func TestCountLookupUsage_NotApplicableChildren(t *testing.T) {
	p := &fakeLSPlanner{sum: lookupsync.Summary{TotalProducts: 4, NotApplicableChildren: 2, SkipCounts: map[string]int{"LOCKED": 1}}}
	h, err := NewCostProductLookupSyncHandler(p)
	require.NoError(t, err)
	resp, err := h.CountLookupUsage(context.Background(), &financev1.CountLookupUsageRequest{LookupMasterCode: "MACHINE", MasterKey: "M1"})
	require.NoError(t, err)
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, int32(4), resp.Data.TotalProducts)
	assert.Equal(t, int32(2), resp.Data.SkipCounts["CHILD_NOT_APPLICABLE"])
	assert.Equal(t, int32(1), resp.Data.SkipCounts["LOCKED"])
	assert.Equal(t, "M1", p.scope.MasterRef.MasterKey)

	resp, _ = h.CountLookupUsage(context.Background(), &financev1.CountLookupUsageRequest{})
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "400", resp.Base.StatusCode)
}

func TestPreviewLookupSync_Paging(t *testing.T) {
	p := &fakeLSPlanner{items: lsItems(5), sum: lookupsync.Summary{TotalProducts: 5, Fingerprint: "fp"}}
	h, _ := NewCostProductLookupSyncHandler(p)
	req := &financev1.PreviewLookupSyncRequest{Scope: masterScope(), Pagination: &commonv1.PaginationRequest{Page: 2, PageSize: 2}}
	resp, err := h.PreviewLookupSync(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int64(3), resp.Items[0].ProductSysId)
	assert.Equal(t, int64(5), resp.Pagination.TotalItems)
	assert.Equal(t, int32(3), resp.Pagination.TotalPages)
	assert.Equal(t, "fp", resp.Summary.PlanFingerprint)

	// default size 20, cap 100
	resp, _ = h.PreviewLookupSync(context.Background(), &financev1.PreviewLookupSyncRequest{Scope: masterScope()})
	assert.Len(t, resp.Items, 5)
	assert.Equal(t, int32(20), resp.Pagination.PageSize)
	resp, _ = h.PreviewLookupSync(context.Background(), &financev1.PreviewLookupSyncRequest{
		Scope: masterScope(), Pagination: &commonv1.PaginationRequest{Page: 1, PageSize: 500}})
	assert.Equal(t, int32(100), resp.Pagination.PageSize)
}

func TestPreviewLookupSync_OnlyActionable(t *testing.T) {
	p := &fakeLSPlanner{items: lsItems(5), sum: lookupsync.Summary{}}
	h, _ := NewCostProductLookupSyncHandler(p)
	resp, err := h.PreviewLookupSync(context.Background(), &financev1.PreviewLookupSyncRequest{Scope: masterScope(), OnlyActionable: true})
	require.NoError(t, err)
	require.Len(t, resp.Items, 3) // indexes 0,2,4
	assert.Equal(t, int64(3), resp.Pagination.TotalItems)
	assert.Equal(t, "SET", resp.Items[0].Changes[0].ChangeKind)
	assert.Equal(t, "2", resp.Items[0].Changes[0].NewValue)

	resp, _ = h.PreviewLookupSync(context.Background(), &financev1.PreviewLookupSyncRequest{Scope: masterScope()})
	assert.Equal(t, "LOCKED", resp.Items[1].SkipReason)
}

func TestApplyLookupSync(t *testing.T) {
	ctx := context.Background()
	p := &fakeLSPlanner{sum: lookupsync.Summary{TotalProducts: 2, Fingerprint: "fp"}}
	h, _ := NewCostProductLookupSyncHandler(p)

	// nil enqueuer -> 503
	resp, err := h.ApplyLookupSync(ctx, &financev1.ApplyLookupSyncRequest{Scope: masterScope(), PlanFingerprint: "fp"})
	require.NoError(t, err)
	assert.Equal(t, "503", resp.Base.StatusCode)

	enq := &fakeLSEnqueuer{}
	h.WithEnqueuer(enq)

	// drift -> 409 + summary, no enqueue
	resp, _ = h.ApplyLookupSync(ctx, &financev1.ApplyLookupSyncRequest{Scope: masterScope(), PlanFingerprint: "old"})
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "409", resp.Base.StatusCode)
	assert.Equal(t, "fp", resp.Summary.PlanFingerprint)
	assert.Zero(t, enq.called)

	// missing fingerprint -> 400
	resp, _ = h.ApplyLookupSync(ctx, &financev1.ApplyLookupSyncRequest{Scope: masterScope()})
	assert.Equal(t, "400", resp.Base.StatusCode)

	// match -> enqueue
	resp, _ = h.ApplyLookupSync(ctx, &financev1.ApplyLookupSyncRequest{Scope: masterScope(), PlanFingerprint: "fp"})
	assert.True(t, resp.Base.IsSuccess)
	assert.Equal(t, 1, enq.called)
	assert.Equal(t, "fp", enq.fingerprint)
	assert.Equal(t, "j1", resp.Data.JobId)
	assert.Equal(t, int32(3), resp.Data.TotalChildren)

	// planner error
	p.err = errors.New("boom")
	resp, _ = h.ApplyLookupSync(ctx, &financev1.ApplyLookupSyncRequest{Scope: masterScope(), PlanFingerprint: "fp"})
	assert.False(t, resp.Base.IsSuccess)
}

func TestLookupSyncJobRPCs_NilTracker(t *testing.T) {
	h, _ := NewCostProductLookupSyncHandler(&fakeLSPlanner{})
	s, err := h.GetLookupSyncJobStatus(context.Background(), &financev1.GetLookupSyncJobStatusRequest{JobId: "x"})
	require.NoError(t, err)
	assert.Equal(t, "501", s.Base.StatusCode)
	f, _ := h.ListLookupSyncJobFailures(context.Background(), &financev1.ListLookupSyncJobFailuresRequest{JobId: "x"})
	assert.Equal(t, "501", f.Base.StatusCode)
}

func TestNewCostProductLookupSyncHandler_NilPlanner(t *testing.T) {
	_, err := NewCostProductLookupSyncHandler(nil)
	assert.Error(t, err)
}
