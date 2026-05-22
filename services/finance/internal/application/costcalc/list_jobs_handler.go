package costcalc

import (
	"context"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// ListJobsQuery describes filters for paginated job listing.
type ListJobsQuery struct {
	Period      string
	CalcType    costcalcdom.CalculationType
	Status      costcalcdom.JobStatus
	TriggeredBy string
	Page        int
	PageSize    int
}

// ListJobsResult is the paginated list result.
type ListJobsResult struct {
	Items    []*costcalcdom.Job
	Total    int
	Page     int
	PageSize int
}

// ListJobsHandler returns a paginated, filtered list of calc jobs.
type ListJobsHandler struct {
	svc *Service
}

// NewListJobsHandler constructs the handler.
func NewListJobsHandler(svc *Service) *ListJobsHandler {
	return &ListJobsHandler{svc: svc}
}

// Handle executes the query.
func (h *ListJobsHandler) Handle(ctx context.Context, q ListJobsQuery) (*ListJobsResult, error) {
	page, size := normalizePagination(q.Page, q.PageSize)
	filter := costcalcdom.JobFilter(q)
	filter.Page = page
	filter.PageSize = size
	items, total, err := h.svc.jobRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return &ListJobsResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// normalizePagination clamps page to >=1 and pageSize to [1,100], defaulting
// missing values to 1/20.
func normalizePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
