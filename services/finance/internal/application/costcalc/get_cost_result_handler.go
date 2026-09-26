package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// GetCostResultQuery selects the active result for (product, period, calcType).
type GetCostResultQuery struct {
	ProductSysID int64
	Period       string
	CalcType     costcalcdom.CalculationType
}

// CostResultView enriches the plain domain Result with the same display
// fields the cross-product list view resolves via SQL joins (item/shade
// code+name, primary RM, full RM breakdown) — see ResultDisplay. It embeds
// *costcalcdom.Result anonymously so existing callers of e.g. result.Status()
// keep compiling unchanged.
type CostResultView struct {
	*costcalcdom.Result
	ResultDisplay
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
func (h *GetCostResultHandler) Handle(ctx context.Context, q GetCostResultQuery) (*CostResultView, error) {
	if q.ProductSysID <= 0 {
		return nil, errors.New(errMsgProductIDPositive)
	}
	if len(q.Period) != 6 {
		return nil, errors.New(errMsgPeriodFormat)
	}
	result, err := h.svc.resultRepo.GetActive(ctx, q.ProductSysID, q.Period, q.CalcType)
	if err != nil {
		return nil, err
	}
	var detail []RMCostDetail
	if err := decodeJSONBlob(result.RMCostDetail(), &detail); err != nil {
		return nil, fmt.Errorf("decode rm_cost_detail: %w", err)
	}
	disp, err := h.svc.resolveResultDisplay(ctx, q.ProductSysID, detail)
	if err != nil {
		return nil, fmt.Errorf("resolve result display: %w", err)
	}
	return &CostResultView{Result: result, ResultDisplay: disp}, nil
}
