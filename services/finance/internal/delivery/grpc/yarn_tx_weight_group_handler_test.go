package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

func validYtwgRules() []*financev1.YarnTxWeightRule {
	return []*financev1.YarnTxWeightRule{
		{Grade: financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_AE, Mode: financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_LESS_BY, Value: 0.5},
		{Grade: financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_B, Mode: financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_FIXED, Value: 2.5},
	}
}

// TestYarnTxWeightGroupRequestValidation exercises the buf.validate rules of
// the group create/update requests at runtime, including the CEL
// "rules.unique_grade" expression (compiled lazily by protovalidate, so a bad
// expression would only surface here, not at build time).
func TestYarnTxWeightGroupRequestValidation(t *testing.T) {
	v, err := NewValidationHelper()
	require.NoError(t, err)

	valid := &financev1.CreateYarnTxWeightGroupRequest{
		Code: "TTY", Name: "Twisted Yarn", ProductTypeIds: []int32{1, 2}, Rules: validYtwgRules(),
	}
	assert.Nil(t, v.ValidateRequest(valid), "valid create must pass")

	dupGrade := &financev1.CreateYarnTxWeightGroupRequest{
		Code: "TTY", Name: "Twisted Yarn", ProductTypeIds: []int32{1},
		Rules: append(validYtwgRules(), &financev1.YarnTxWeightRule{
			Grade: financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_AE, Mode: financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_FIXED, Value: 1,
		}),
	}
	assert.NotNil(t, v.ValidateRequest(dupGrade), "duplicate grade must fail")

	dupType := &financev1.CreateYarnTxWeightGroupRequest{
		Code: "TTY", Name: "Twisted Yarn", ProductTypeIds: []int32{1, 1}, Rules: validYtwgRules(),
	}
	assert.NotNil(t, v.ValidateRequest(dupType), "duplicate product type must fail")

	noTypes := &financev1.CreateYarnTxWeightGroupRequest{Code: "TTY", Name: "x", Rules: validYtwgRules()}
	assert.NotNil(t, v.ValidateRequest(noTypes), "no product type must fail")

	badCode := &financev1.CreateYarnTxWeightGroupRequest{
		Code: "tty", Name: "x", ProductTypeIds: []int32{1}, Rules: validYtwgRules(),
	}
	assert.NotNil(t, v.ValidateRequest(badCode), "lowercase code must fail")

	upd := &financev1.UpdateYarnTxWeightGroupRequest{
		GroupId: "7b0e2f4a-9f58-4f0a-9d7e-2f1f9c3f5a11", Code: "DTY", Name: "DTY",
		ProductTypeIds: []int32{3}, Rules: validYtwgRules(),
	}
	assert.Nil(t, v.ValidateRequest(upd), "valid update must pass")

	upd.Rules = append(upd.Rules, &financev1.YarnTxWeightRule{
		Grade: financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_B, Mode: financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_FIXED, Value: 1,
	})
	assert.NotNil(t, v.ValidateRequest(upd), "duplicate grade on update must fail")
}

// TestCreateYarnTxWeight_DeprecatedReturnsFailedPrecondition pins the design
// decision that the per product type Create is closed since 000536: a
// group-less rule row would be ignored by the engine.
func TestCreateYarnTxWeight_DeprecatedReturnsFailedPrecondition(t *testing.T) {
	h, err := NewYarnTxWeightHandler(nil)
	require.NoError(t, err)

	resp, err := h.CreateYarnTxWeight(t.Context(), &financev1.CreateYarnTxWeightRequest{
		ProductTypeId: 1,
		Grade:         financev1.YarnTxWeightGrade_YARN_TX_WEIGHT_GRADE_AE,
		Mode:          financev1.YarnTxWeightMode_YARN_TX_WEIGHT_MODE_LESS_BY,
		Value:         0.5,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "412", resp.Base.StatusCode)
	assert.Contains(t, resp.Base.Message, "YarnTxWeightGroupService")
	assert.Nil(t, resp.Data)
}
