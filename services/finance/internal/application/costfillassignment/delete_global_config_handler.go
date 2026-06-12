package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// DeleteGlobalConfigCommand specifies which level's global config to deactivate.
type DeleteGlobalConfigCommand struct {
	RouteLevel int32
}

// DeleteGlobalConfigHandler deactivates the active global config for a route level.
type DeleteGlobalConfigHandler struct {
	repo domain.ConfigRepository
}

// NewDeleteGlobalConfigHandler constructs the handler.
func NewDeleteGlobalConfigHandler(repo domain.ConfigRepository) *DeleteGlobalConfigHandler {
	return &DeleteGlobalConfigHandler{repo: repo}
}

// Handle validates and delegates the delete.
func (h *DeleteGlobalConfigHandler) Handle(ctx context.Context, cmd DeleteGlobalConfigCommand) error {
	if cmd.RouteLevel < 1 {
		return fmt.Errorf("route level must be >= 1")
	}
	if err := h.repo.DeleteGlobal(ctx, cmd.RouteLevel); err != nil {
		return fmt.Errorf("delete global config level %d: %w", cmd.RouteLevel, err)
	}
	return nil
}
