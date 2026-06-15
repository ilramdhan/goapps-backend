package costproductrequest

import (
	"context"
	"fmt"
)

// ParamValueRow is one parameter cell for a product at a route level.
type ParamValueRow struct {
	ParamID      string
	ParamCode    string
	ParamName    string
	DataType     string
	HasValue     bool
	ValueNumeric string
	ValueText    string
	ValueFlag    bool
	UOMCode      string
	IsRequired   bool
}

// LevelSummaryRow groups params for one fill task level.
type LevelSummaryRow struct {
	RouteLevel     int32
	TaskStatus     string
	FilledByUserID string
	FilledAt       string
	FilledParams   int32
	TotalParams    int32
	Params         []ParamValueRow
}

// ProductSummaryRow is one product in the param summary.
type ProductSummaryRow struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	Levels       []LevelSummaryRow
}

// ParamSummaryRepository fetches the full param summary for a request.
type ParamSummaryRepository interface {
	GetParamSummary(ctx context.Context, requestID int64) ([]ProductSummaryRow, error)
}

// GetParamSummaryHandler fetches the param summary for a CPR.
type GetParamSummaryHandler struct {
	repo ParamSummaryRepository
}

// NewGetParamSummaryHandler constructs the handler.
func NewGetParamSummaryHandler(repo ParamSummaryRepository) *GetParamSummaryHandler {
	return &GetParamSummaryHandler{repo: repo}
}

// GetParamSummaryQuery is the input.
type GetParamSummaryQuery struct {
	RequestID int64
}

// Handle executes the query and returns products, totalParams, filledParams.
func (h *GetParamSummaryHandler) Handle(ctx context.Context, q GetParamSummaryQuery) ([]ProductSummaryRow, int32, int32, error) {
	if q.RequestID <= 0 {
		return nil, 0, 0, fmt.Errorf("invalid request ID")
	}
	products, err := h.repo.GetParamSummary(ctx, q.RequestID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("get param summary: %w", err)
	}
	var total, filled int32
	for _, p := range products {
		for _, l := range p.Levels {
			total += l.TotalParams
			filled += l.FilledParams
		}
	}
	return products, total, filled, nil
}
