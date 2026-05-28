package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// UpdateCommand is the payload for UpdateHandler (all mutable fields are optional).
type UpdateCommand struct {
	JobID           uuid.UUID
	ScheduleCron    *string
	OracleProcedure *string
	Config          map[string]any
	IsActive        *bool
	UpdatedBy       uuid.UUID
}

// UpdateHandler applies partial mutations to an existing ETL job.
type UpdateHandler struct{ repo jobdomain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r jobdomain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle validates and persists the mutation.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*jobdomain.Job, error) {
	if cmd.ScheduleCron != nil {
		if err := jobdomain.ValidateCronExpression(*cmd.ScheduleCron); err != nil {
			return nil, fmt.Errorf("validate cron: %w", err)
		}
	}
	j, err := h.repo.Update(ctx, jobdomain.UpdateJobParams{
		ID:              cmd.JobID,
		ScheduleCron:    cmd.ScheduleCron,
		OracleProcedure: cmd.OracleProcedure,
		Config:          cmd.Config,
		IsActive:        cmd.IsActive,
		UpdatedBy:       cmd.UpdatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("update bi job: %w", err)
	}
	return j, nil
}
