package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// ListCostHistoryQuery filters cost result history for a product.
type ListCostHistoryQuery struct {
	ProductSysID int64
	CalcType     costcalcdom.CalculationType
	Page         int
	PageSize     int
}

// ListCostHistoryResult is the paginated history list result.
type ListCostHistoryResult struct {
	Items    []*costcalcdom.Result
	Total    int
	Page     int
	PageSize int
}

// ListCostHistoryHandler returns paginated result rows (including SUPERSEDED).
type ListCostHistoryHandler struct {
	svc *Service
}

// NewListCostHistoryHandler constructs the handler.
func NewListCostHistoryHandler(svc *Service) *ListCostHistoryHandler {
	return &ListCostHistoryHandler{svc: svc}
}

// Handle executes the query.
func (h *ListCostHistoryHandler) Handle(ctx context.Context, q ListCostHistoryQuery) (*ListCostHistoryResult, error) {
	if q.ProductSysID <= 0 {
		return nil, errors.New(errMsgProductIDPositive)
	}
	page, size := normalizePagination(q.Page, q.PageSize)
	items, total, err := h.svc.resultRepo.ListHistory(ctx, q.ProductSysID, q.CalcType, page, size)
	if err != nil {
		return nil, fmt.Errorf("list cost history: %w", err)
	}
	return &ListCostHistoryResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}
