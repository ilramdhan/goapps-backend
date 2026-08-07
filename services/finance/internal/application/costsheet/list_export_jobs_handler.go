package costsheet

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// ListExportJobsQuery describes filters/pagination for the recent-exports
// history listing. Page/PageSize follow the same 1-based, default-20,
// max-100 convention as the other paginated list handlers in this service.
type ListExportJobsQuery struct {
	Period   string
	Page     int
	PageSize int
}

// ListExportJobsResult is the paginated list result.
type ListExportJobsResult struct {
	Items    []*job.Execution
	Total    int64
	Page     int
	PageSize int
}

// ListExportJobsHandler returns a paginated, newest-first list of recent
// product cost sheet export jobs — both standalone single-product jobs and
// batch-tracking parents — so users can find past exports without relying on
// a one-time notification link. Batch children are excluded at the query
// level: they are not independent artifacts a user would look for in this
// history, only their parent is.
type ListExportJobsHandler struct {
	jobRepo job.Repository
}

// NewListExportJobsHandler constructs the handler.
func NewListExportJobsHandler(jobRepo job.Repository) *ListExportJobsHandler {
	return &ListExportJobsHandler{jobRepo: jobRepo}
}

// Handle executes the query.
func (h *ListExportJobsHandler) Handle(ctx context.Context, q ListExportJobsQuery) (*ListExportJobsResult, error) {
	page, pageSize := normalizeExportJobsPagination(q.Page, q.PageSize)

	filter := job.ListFilter{
		JobType:         job.TypeProductCostSheetExport.String(),
		Period:          q.Period,
		Page:            page,
		PageSize:        pageSize,
		ExcludeChildren: true,
	}

	items, total, err := h.jobRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list export jobs: %w", err)
	}

	return &ListExportJobsResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// normalizeExportJobsPagination clamps page to >=1 and pageSize to [1,100],
// defaulting missing values to 1/20 — mirrors costcalc.normalizePagination.
func normalizeExportJobsPagination(page, pageSize int) (int, int) {
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
