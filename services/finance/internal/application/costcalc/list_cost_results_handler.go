package costcalc

import (
	"context"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// ListCostResultsQuery filters active cost results across products.
type ListCostResultsQuery struct {
	Period   string
	CalcType costcalcdom.CalculationType
	Status   string
	Search   string
	Page     int
	PageSize int
}

// ListCostResultsResult is the paginated cross-product result list.
type ListCostResultsResult struct {
	Items          []*costcalcdom.ResultSummary
	Total          int
	Page           int
	PageSize       int
	ResolvedPeriod string
}

// ListCostResultsHandler returns active cost results across products for a
// period (defaulting to the latest period when none is given).
type ListCostResultsHandler struct {
	svc *Service
}

// NewListCostResultsHandler constructs the handler.
func NewListCostResultsHandler(svc *Service) *ListCostResultsHandler {
	return &ListCostResultsHandler{svc: svc}
}

// Handle executes the query.
func (h *ListCostResultsHandler) Handle(ctx context.Context, q ListCostResultsQuery) (*ListCostResultsResult, error) {
	page, size := normalizePagination(q.Page, q.PageSize)
	items, total, period, err := h.svc.resultRepo.ListResults(ctx, costcalcdom.ResultListFilter{
		Period:   q.Period,
		CalcType: q.CalcType,
		Status:   q.Status,
		Search:   q.Search,
		Page:     page,
		PageSize: size,
	})
	if err != nil {
		return nil, fmt.Errorf("list cost results: %w", err)
	}
	return &ListCostResultsResult{
		Items: items, Total: total, Page: page, PageSize: size, ResolvedPeriod: period,
	}, nil
}
