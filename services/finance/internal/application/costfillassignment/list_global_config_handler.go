package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ListGlobalConfigQuery is the query for listing all active global configs.
type ListGlobalConfigQuery struct{}

// ListGlobalConfigResult is the handler result.
type ListGlobalConfigResult struct {
	Configs []*domain.Config
}

// ListGlobalConfigHandler lists all active global assignment configs.
type ListGlobalConfigHandler struct {
	repo domain.ConfigRepository
}

// NewListGlobalConfigHandler constructs the handler.
func NewListGlobalConfigHandler(repo domain.ConfigRepository) *ListGlobalConfigHandler {
	return &ListGlobalConfigHandler{repo: repo}
}

// Handle fetches all active global configs from the repository.
func (h *ListGlobalConfigHandler) Handle(ctx context.Context, _ ListGlobalConfigQuery) (*ListGlobalConfigResult, error) {
	configs, err := h.repo.ListGlobal(ctx)
	if err != nil {
		return nil, fmt.Errorf("list global configs: %w", err)
	}
	return &ListGlobalConfigResult{Configs: configs}, nil
}
