package costcalc

import (
	"context"
	"errors"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// GetCostResultQuery selects the active result for (product, period, calcType).
type GetCostResultQuery struct {
	ProductSysID int64
	Period       string
	CalcType     costcalcdom.CalculationType
}

// GetCostResultHandler returns the currently active (non-SUPERSEDED) cost
// result for a product/period/type triple.
type GetCostResultHandler struct {
	svc *Service
}

// NewGetCostResultHandler constructs the handler.
func NewGetCostResultHandler(svc *Service) *GetCostResultHandler {
	return &GetCostResultHandler{svc: svc}
}

// Handle executes the query.
func (h *GetCostResultHandler) Handle(ctx context.Context, q GetCostResultQuery) (*costcalcdom.Result, error) {
	if q.ProductSysID <= 0 {
		return nil, errors.New(errMsgProductIDPositive)
	}
	if len(q.Period) != 6 {
		return nil, errors.New(errMsgPeriodFormat)
	}
	return h.svc.resultRepo.GetActive(ctx, q.ProductSysID, q.Period, q.CalcType)
}
