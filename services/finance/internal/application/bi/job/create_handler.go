package job

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// CreateCommand is the payload for CreateHandler.
// SourceCode is the human-readable code (e.g. "ERP_ORACLE"); the repository
// resolves it to a UUID via the bi_data_source table.
type CreateCommand struct {
	JobName         string
	SourceCode      string
	TargetType      string
	ScheduleCron    string
	OracleProcedure string
	Config          map[string]any
	IsActive        bool
	CreatedBy       uuid.UUID
}

// CreateHandler creates a new ETL job in the registry.
type CreateHandler struct{ repo jobdomain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r jobdomain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle validates and persists a new ETL job.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*jobdomain.Job, error) {
	if err := jobdomain.ValidateCronExpression(cmd.ScheduleCron); err != nil {
		return nil, fmt.Errorf("validate cron: %w", err)
	}
	j, err := h.repo.Create(ctx, jobdomain.CreateJobParams{
		JobName:         cmd.JobName,
		SourceCode:      cmd.SourceCode,
		TargetType:      cmd.TargetType,
		ScheduleCron:    cmd.ScheduleCron,
		OracleProcedure: cmd.OracleProcedure,
		Config:          cmd.Config,
		IsActive:        cmd.IsActive,
		CreatedBy:       cmd.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create bi job: %w", err)
	}
	return j, nil
}
