package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// DeleteCommand is the payload for DeleteHandler.
type DeleteCommand struct {
	JobID     uuid.UUID
	DeletedBy uuid.UUID
}

// DeleteHandler soft-disables an ETL job (sets is_active=false, preserves logs).
type DeleteHandler struct{ repo jobdomain.Repository }

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(r jobdomain.Repository) *DeleteHandler { return &DeleteHandler{repo: r} }

// Handle soft-disables the job.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	if err := h.repo.Delete(ctx, cmd.JobID, cmd.DeletedBy); err != nil {
		return fmt.Errorf("delete bi job: %w", err)
	}
	return nil
}
