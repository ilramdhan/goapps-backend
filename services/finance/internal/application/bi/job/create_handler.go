package job

import (
	"context"
	"fmt"
	"maps"
	"strings"

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

	// Auto-populate config fields from command fields so the form never needs
	// to manually set "kind" or "source_view" — the convention is:
	//   kind        = "etl_" + lower(replace(target_type, " ", "_"))
	//   source_view = oracle_procedure (MV or SP name; stored for auditability)
	// This means adding a new ETL type only requires:
	//   1. A new BIMVRepository.FetchXxx() method in the oracle package.
	//   2. A new case in MVLoader.Load() (bietl package).
	//   3. A job in the admin panel with the matching target_type.
	// No changes to TriggerHandler, form, or dropdown are needed.
	cfg := make(map[string]any, len(cmd.Config)+3)
	maps.Copy(cfg, cmd.Config)
	if _, hasKind := cfg["kind"]; !hasKind && cmd.TargetType != "" {
		kind := "etl_" + strings.ToLower(strings.ReplaceAll(cmd.TargetType, " ", "_"))
		cfg["kind"] = kind
	}
	if _, hasView := cfg["source_view"]; !hasView && cmd.OracleProcedure != "" {
		cfg["source_view"] = cmd.OracleProcedure
	}
	if _, hasType := cfg["target_type"]; !hasType && cmd.TargetType != "" {
		cfg["target_type"] = cmd.TargetType
	}

	j, err := h.repo.Create(ctx, jobdomain.CreateJobParams{
		JobName:         cmd.JobName,
		SourceCode:      cmd.SourceCode,
		TargetType:      cmd.TargetType,
		ScheduleCron:    cmd.ScheduleCron,
		OracleProcedure: cmd.OracleProcedure,
		Config:          cfg,
		IsActive:        cmd.IsActive,
		CreatedBy:       cmd.CreatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create bi job: %w", err)
	}
	return j, nil
}
