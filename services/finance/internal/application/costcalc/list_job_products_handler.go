package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// ListJobProductsQuery filters per-product rows within a job.
type ListJobProductsQuery struct {
	JobID    int64
	Status   costcalcdom.JobProductStatus
	Page     int
	PageSize int
}

// ListJobProductsResult is the paginated per-product list result.
type ListJobProductsResult struct {
	Items    []*costcalcdom.JobProduct
	Total    int
	Page     int
	PageSize int
}

// ListJobProductsHandler returns paginated per-product rows for a job.
type ListJobProductsHandler struct {
	svc *Service
}

// NewListJobProductsHandler constructs the handler.
func NewListJobProductsHandler(svc *Service) *ListJobProductsHandler {
	return &ListJobProductsHandler{svc: svc}
}

// Handle executes the query.
func (h *ListJobProductsHandler) Handle(ctx context.Context, q ListJobProductsQuery) (*ListJobProductsResult, error) {
	if q.JobID <= 0 {
		return nil, errors.New(errMsgJobIDPositive)
	}
	page, size := normalizePagination(q.Page, q.PageSize)
	filter := costcalcdom.JobProductFilter{
		Status:   q.Status,
		Page:     page,
		PageSize: size,
	}
	items, total, err := h.svc.productRepo.ListByJob(ctx, q.JobID, filter)
	if err != nil {
		return nil, fmt.Errorf("list job products: %w", err)
	}
	return &ListJobProductsResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}
