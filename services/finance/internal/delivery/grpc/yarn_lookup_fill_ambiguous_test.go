package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestFillErrorToBaseResponse_AmbiguousMBHead_Is409(t *testing.T) {
	base := fillErrorToBaseResponse(lookupsync.MasterMBHead, fmt.Errorf("%w: \"X\" matches 2 heads", mbhead.ErrMBCostingAmbiguous))
	assert.False(t, base.GetIsSuccess())
	assert.Equal(t, "409", base.GetStatusCode())
	assert.Equal(t, "MB Head name is ambiguous: more than one MB head has this name", base.GetMessage())
}

type dupHeadRepo struct {
	mbhead.Repository
}

func (dupHeadRepo) CountByMBCosting(context.Context, string) (int, error) { return 2, nil }

func TestGetLookupFillValues_AmbiguousMBHead_Conflict(t *testing.T) {
	h := &YarnLookupFillHandler{mbHeadRepo: dupHeadRepo{}}
	resp := h.fill(context.Background(), lookupsync.MasterMBHead, "DUP", "MB_CODE")
	assert.Equal(t, "409", resp.GetBase().GetStatusCode())
}
