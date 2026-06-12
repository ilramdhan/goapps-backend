package costfillassignment

import (
	"context"
	"fmt"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// ClaimTaskCommand carries claim request data.
type ClaimTaskCommand struct {
	TaskID int64
	UserID string
}

// ClaimTaskHandler atomically claims an ACTIVE fill task for a user.
type ClaimTaskHandler struct {
	repo domain.TaskRepository
}

// NewClaimTaskHandler constructs the handler.
func NewClaimTaskHandler(repo domain.TaskRepository) *ClaimTaskHandler {
	return &ClaimTaskHandler{repo: repo}
}

// Handle atomically claims the task. Returns ErrAlreadyClaimed if someone else got it first.
func (h *ClaimTaskHandler) Handle(ctx context.Context, cmd ClaimTaskCommand) error {
	if cmd.TaskID <= 0 {
		return fmt.Errorf("task ID must be > 0")
	}
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	ok, err := h.repo.Claim(ctx, cmd.TaskID, cmd.UserID)
	if err != nil {
		return fmt.Errorf("claim task %d: %w", cmd.TaskID, err)
	}
	if !ok {
		return domain.ErrAlreadyClaimed
	}
	return nil
}
