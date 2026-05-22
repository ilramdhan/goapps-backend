package costcalc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// GetCostBreakdownQuery selects the breakdown for an active cost result.
type GetCostBreakdownQuery struct {
	ProductSysID int64
	Period       string
	CalcType     costcalcdom.CalculationType
}

// CostBreakdownView combines the active Result with its decoded JSONB blobs.
type CostBreakdownView struct {
	Result        *costcalcdom.Result
	CostByLevel   []LevelContribution
	RMCostDetail  []RMCostDetail
	ParamSnapshot map[string]float64
	FormulaTrace  []FormulaEvalTrace
}

// GetCostBreakdownHandler loads the active result and decodes every JSONB blob
// into typed slices/maps. Empty/nil blobs decode to empty containers (not nil)
// so callers don't need nil-checks.
type GetCostBreakdownHandler struct {
	svc *Service
}

// NewGetCostBreakdownHandler constructs the handler.
func NewGetCostBreakdownHandler(svc *Service) *GetCostBreakdownHandler {
	return &GetCostBreakdownHandler{svc: svc}
}

// Handle executes the query.
func (h *GetCostBreakdownHandler) Handle(ctx context.Context, q GetCostBreakdownQuery) (*CostBreakdownView, error) {
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

	view := &CostBreakdownView{
		Result:        result,
		CostByLevel:   []LevelContribution{},
		RMCostDetail:  []RMCostDetail{},
		ParamSnapshot: map[string]float64{},
		FormulaTrace:  []FormulaEvalTrace{},
	}
	if err := decodeJSONBlob(result.CostByLevel(), &view.CostByLevel); err != nil {
		return nil, fmt.Errorf("decode cost_by_level: %w", err)
	}
	if err := decodeJSONBlob(result.RMCostDetail(), &view.RMCostDetail); err != nil {
		return nil, fmt.Errorf("decode rm_cost_detail: %w", err)
	}
	if err := decodeJSONBlob(result.ParamSnapshot(), &view.ParamSnapshot); err != nil {
		return nil, fmt.Errorf("decode param_snapshot: %w", err)
	}
	if err := decodeJSONBlob(result.FormulaTrace(), &view.FormulaTrace); err != nil {
		return nil, fmt.Errorf("decode formula_trace: %w", err)
	}
	return view, nil
}

// decodeJSONBlob is a nil-safe wrapper: empty payloads are a no-op so the
// destination keeps its initialized empty value instead of becoming nil.
func decodeJSONBlob(payload []byte, dest any) error {
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, dest)
}
