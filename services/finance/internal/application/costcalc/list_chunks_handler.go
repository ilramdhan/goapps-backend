package costcalc

import (
	"context"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// ListChunksQuery filters chunks within a single job.
type ListChunksQuery struct {
	JobID    int64
	WaveNo   *int
	Status   *costcalcdom.ChunkStatus
	Page     int
	PageSize int
}

// ListChunksResult is the paginated chunk list result.
type ListChunksResult struct {
	Items    []*costcalcdom.Chunk
	Total    int
	Page     int
	PageSize int
}

// ListChunksHandler returns paginated chunks for a job with optional wave/status filters.
type ListChunksHandler struct {
	svc *Service
}

// NewListChunksHandler constructs the handler.
func NewListChunksHandler(svc *Service) *ListChunksHandler {
	return &ListChunksHandler{svc: svc}
}

// Handle executes the query.
func (h *ListChunksHandler) Handle(ctx context.Context, q ListChunksQuery) (*ListChunksResult, error) {
	if q.JobID <= 0 {
		return nil, errors.New(errMsgJobIDPositive)
	}
	page, size := normalizePagination(q.Page, q.PageSize)
	items, total, err := h.svc.chunkRepo.ListByJob(ctx, q.JobID, q.WaveNo, q.Status, page, size)
	if err != nil {
		return nil, fmt.Errorf("list chunks: %w", err)
	}
	return &ListChunksResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}
