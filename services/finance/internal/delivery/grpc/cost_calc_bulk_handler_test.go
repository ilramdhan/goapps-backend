package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

func TestBulkFailureReason_MapsKnownAndHidesInternal(t *testing.T) {
	t.Parallel()
	cases := map[error]string{
		fmt.Errorf("x: %w", costcalcdom.ErrPeriodLocked):                 "period is locked",
		fmt.Errorf("x: %w", costcalcdom.ErrCostInvalidStatus):            "status does not allow this transition (already done or superseded)",
		fmt.Errorf("x: %w", costcalc.ErrMBCostNotManuallyTransitionable): costcalc.ErrMBCostNotManuallyTransitionable.Error(),
		context.DeadlineExceeded:                                         "not processed: request timed out, retry the remaining rows",
		errors.New("pq: connection refused at 10.0.0.1"):                 "internal error",
	}
	for err, want := range cases {
		assert.Equal(t, want, bulkFailureReason(err), err.Error())
	}
}

func TestBulkVerifyCostResults_FilterWithoutPeriod_Returns400(t *testing.T) {
	t.Parallel()
	svc := costcalc.NewService(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h := &CostCalcHandler{bulkVerifyH: costcalc.NewBulkVerifyHandler(svc, costcalc.NewVerifyCostHandler(svc))}
	ctx := context.WithValue(context.Background(), AuthUserIDKey, "u1")
	resp, err := h.BulkVerifyCostResults(ctx, &financev1.BulkVerifyCostResultsRequest{Selection: &financev1.CostResultBulkSelection{}})
	require.NoError(t, err)
	assert.False(t, resp.GetBase().GetIsSuccess())
	assert.Equal(t, "400", resp.GetBase().GetStatusCode())
}
