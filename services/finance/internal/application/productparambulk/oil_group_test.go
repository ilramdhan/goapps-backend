package productparambulk_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/productparambulk"
	cppdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

type oilPolicyStub struct {
	rules map[int64]*cppdomain.OilGroupRule
}

func (s *oilPolicyStub) RuleForProduct(_ context.Context, id int64) (*cppdomain.OilGroupRule, error) {
	return s.rules[id], nil
}

func (s *oilPolicyStub) RulesForProducts(_ context.Context, ids []int64) (map[int64]*cppdomain.OilGroupRule, error) {
	out := map[int64]*cppdomain.OilGroupRule{}
	for _, id := range ids {
		if r, ok := s.rules[id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

type paramCodeStub struct{ codes map[uuid.UUID]string }

func (s *paramCodeStub) GetParamCodeByID(_ context.Context, id uuid.UUID) (string, error) {
	return s.codes[id], nil
}

func TestRequestBulkEditHandler_OilName(t *testing.T) {
	t.Parallel()
	oilParam := uuid.New()
	otherParam := uuid.New()
	codes := &paramCodeStub{codes: map[uuid.UUID]string{oilParam: "OIL_NAME", otherParam: "COLOR"}}
	policy := &oilPolicyStub{rules: map[int64]*cppdomain.OilGroupRule{
		1: {TypeCode: "PTY", OilClass: "PTY", Allowed: []string{"202006101"}, Default: "202006101"},
		2: {TypeCode: "POY", OilClass: "POY", Allowed: []string{"202006077"}, Default: "202006077"},
		// 3: no oil class
	}}
	upsert := func(param uuid.UUID, v string) productparambulk.OperationDTO {
		return productparambulk.OperationDTO{Kind: productparambulk.OpUpsertValue, ParamID: param.String(), ValueText: &v}
	}

	t.Run("disallowed for one product rejects whole request with per-product list", func(t *testing.T) {
		t.Parallel()
		repo := &jobRepoMock{}
		pub := &publisherMock{}
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil).WithOilGroupPolicy(policy, codes)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1, 2, 3},
			Operations:    []productparambulk.OperationDTO{upsert(oilParam, "202006101")},
			CreatedBy:     "admin",
		})
		require.ErrorIs(t, err, cppdomain.ErrOilGroupNotAllowed)
		assert.Contains(t, err.Error(), "product 2:")
		assert.NotContains(t, err.Error(), "product 1:")
		assert.NotContains(t, err.Error(), "product 3:")
		repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("allowed for all products is queued", func(t *testing.T) {
		t.Parallel()
		repo := &jobRepoMock{}
		pub := &publisherMock{}
		repo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
		repo.On("CreateChildren", mock.Anything, mock.Anything).Return(nil).Once()
		pub.On("PublishProductParamBulk", mock.Anything, mock.Anything, mock.Anything, mock.Anything, false, "admin").Return(nil)
		h := productparambulk.NewRequestBulkEditHandler(repo, pub, nil).WithOilGroupPolicy(policy, codes)
		_, err := h.Handle(context.Background(), productparambulk.RequestBulkEditCommand{
			ProductSysIDs: []int64{1, 3},
			Operations:    []productparambulk.OperationDTO{upsert(oilParam, "202006101"), upsert(otherParam, "X")},
			CreatedBy:     "admin",
		})
		require.NoError(t, err)
		repo.AssertExpectations(t)
	})
}
