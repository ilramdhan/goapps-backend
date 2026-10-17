package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

type fakeOverrideRepo struct {
	upsertErr, deleteErr error
	gotApply             bool
	gotDelete            domain.DeleteRMDenierOverrideInput
}

func (f *fakeOverrideRepo) ListRMDenierOverrides(_ context.Context, pid int64) ([]*domain.RMDenierOverride, error) {
	return []*domain.RMDenierOverride{domain.ReconstructRMDenierOverride(domain.RMDenierOverrideRecord{
		ID: 1, ProductSysID: pid, RMType: "PRODUCT", RMProductSysID: 5, RMProductCode: "P5", Denier: 70, Reason: "r",
	})}, nil
}

func (f *fakeOverrideRepo) ListDirectDuplicates(context.Context, int64) ([]domain.DuplicateRef, error) {
	return []domain.DuplicateRef{{ProductSysID: 99, ProductCode: "D99", ProductName: "dup"}}, nil
}

func (f *fakeOverrideRepo) UpsertRMDenierOverride(_ context.Context, o *domain.RMDenierOverride, apply bool, _ string) (*domain.RMDenierOverride, []int64, error) {
	f.gotApply = apply
	if f.upsertErr != nil {
		return nil, nil, f.upsertErr
	}
	if apply {
		return o, []int64{99}, nil
	}
	return o, nil, nil
}

func (f *fakeOverrideRepo) DeleteRMDenierOverride(_ context.Context, in domain.DeleteRMDenierOverrideInput) (int, []int64, error) {
	f.gotDelete = in
	if f.deleteErr != nil {
		return 0, nil, f.deleteErr
	}
	if in.ApplyToDuplicates {
		return 2, []int64{99}, nil
	}
	return 1, nil, nil
}

func newOverrideHandler(t *testing.T, r *fakeOverrideRepo) *CostProductMasterHandler {
	t.Helper()
	v, err := NewValidationHelper()
	require.NoError(t, err)
	h := &CostProductMasterHandler{validation: v}
	h.WithRMDenierOverrides(r)
	return h
}

func TestRMDenierOverride_List(t *testing.T) {
	h := newOverrideHandler(t, &fakeOverrideRepo{})
	resp, err := h.ListRMDenierOverrides(context.Background(), &financev1.ListRMDenierOverridesRequest{ProductSysId: 10})
	require.NoError(t, err)
	assert.True(t, resp.GetBase().GetIsSuccess())
	require.Len(t, resp.GetData(), 1)
	assert.Equal(t, "P5", resp.GetData()[0].GetRmProductCode())
	require.Len(t, resp.GetDuplicateProducts(), 1)
	assert.Equal(t, int64(99), resp.GetDuplicateProducts()[0].GetProductSysId())
}

func TestRMDenierOverride_UpsertPassthroughAndErrors(t *testing.T) {
	repo := &fakeOverrideRepo{}
	h := newOverrideHandler(t, repo)
	req := &financev1.UpsertRMDenierOverrideRequest{ProductSysId: 10, RmType: "PRODUCT", RmProductSysId: 5, Denier: 70, Reason: "why", ApplyToDuplicates: true}
	resp, err := h.UpsertRMDenierOverride(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.GetBase().GetIsSuccess())
	assert.True(t, repo.gotApply)
	assert.Equal(t, []int64{99}, resp.GetAppliedProductSysIds())

	req.ApplyToDuplicates = false
	resp, _ = h.UpsertRMDenierOverride(context.Background(), req)
	assert.False(t, repo.gotApply)
	assert.Empty(t, resp.GetAppliedProductSysIds())

	// domain validation: owner as RM -> 400 with validation error
	bad := &financev1.UpsertRMDenierOverrideRequest{ProductSysId: 10, RmType: "PRODUCT", RmProductSysId: 10, Denier: 70, Reason: "why"}
	resp, _ = h.UpsertRMDenierOverride(context.Background(), bad)
	assert.Equal(t, "400", resp.GetBase().GetStatusCode())
	assert.NotEmpty(t, resp.GetBase().GetValidationErrors())

	repo.upsertErr = domain.ErrOverrideRMGroupNotFound
	resp, _ = h.UpsertRMDenierOverride(context.Background(), req)
	assert.Equal(t, "404", resp.GetBase().GetStatusCode())
	repo.upsertErr = domain.ErrNotFound
	resp, _ = h.UpsertRMDenierOverride(context.Background(), req)
	assert.Equal(t, "404", resp.GetBase().GetStatusCode())
}

func TestRMDenierOverride_Delete(t *testing.T) {
	repo := &fakeOverrideRepo{}
	h := newOverrideHandler(t, repo)
	resp, err := h.DeleteRMDenierOverride(context.Background(), &financev1.DeleteRMDenierOverrideRequest{
		ProductSysId: 10, RmType: "GROUP", RmGroupCode: "G1", ApplyToDuplicates: true,
	})
	require.NoError(t, err)
	assert.True(t, resp.GetBase().GetIsSuccess())
	assert.Equal(t, int32(2), resp.GetDeletedCount())
	assert.Equal(t, []int64{99}, resp.GetAppliedProductSysIds())
	assert.Equal(t, "GROUP", repo.gotDelete.RMType)
	assert.True(t, repo.gotDelete.ApplyToDuplicates)

	repo.deleteErr = domain.ErrNotFound
	resp, _ = h.DeleteRMDenierOverride(context.Background(), &financev1.DeleteRMDenierOverrideRequest{ProductSysId: 10, RmType: "GROUP", RmGroupCode: "G1"})
	assert.Equal(t, "404", resp.GetBase().GetStatusCode())
}

func TestRMDenierOverride_Permissions(t *testing.T) {
	base := "/finance.v1.CostProductMasterService/"
	assert.Equal(t, "finance.product.route.view", getRequiredPermission(base+"ListRMDenierOverrides"))
	assert.Equal(t, "finance.product.route.create", getRequiredPermission(base+"UpsertRMDenierOverride"))
	assert.Equal(t, "finance.product.route.create", getRequiredPermission(base+"DeleteRMDenierOverride"))
}
