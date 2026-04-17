package oraclesync

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// ListDataQuery holds the input for listing synced data.
type ListDataQuery struct {
	Page     int
	PageSize int
	Period   string
	ItemCode string
	Search   string
}

// ListDataResult holds the output of listing synced data.
type ListDataResult struct {
	Items []*syncdata.ItemConsStockPO
	Total int64
}

// ListDataHandler retrieves a paginated list of synced item data.
type ListDataHandler struct {
	pgRepo syncdata.PostgresTargetRepository
}

// NewListDataHandler creates a new ListDataHandler.
func NewListDataHandler(pgRepo syncdata.PostgresTargetRepository) *ListDataHandler {
	return &ListDataHandler{pgRepo: pgRepo}
}

// Handle retrieves a paginated list of synced records.
func (h *ListDataHandler) Handle(ctx context.Context, query ListDataQuery) (*ListDataResult, error) {
	filter := syncdata.ListFilter{
		Period:   query.Period,
		ItemCode: query.ItemCode,
		Search:   query.Search,
		Page:     query.Page,
		PageSize: query.PageSize,
	}

	items, total, err := h.pgRepo.ListItemConsStockPO(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list synced data: %w", err)
	}

	return &ListDataResult{
		Items: items,
		Total: total,
	}, nil
}
