// Package workflowtemplate contains use cases for workflow templates.
package workflowtemplate

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowtemplate"
)

// StepInput is the per-step payload (used by Create + Update).
type StepInput struct {
	StepNo                  int
	StepName                string
	ResolutionType          string
	ResolutionValue         string
	SLAHours                int
	AllowReject             bool
	AllowReassign           bool
	RequirePasswordOnUnlock bool
	RejectToStepNo          int
}

// CreateCommand creates a brand-new template (version 1, inactive).
type CreateCommand struct {
	Kind        string
	Name        string
	Description string
	Steps       []StepInput
	CreatedBy   string
}

// CreateHandler creates a new template.
type CreateHandler struct{ repo workflowtemplate.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r workflowtemplate.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*workflowtemplate.Template, error) {
	steps, err := buildSteps(cmd.Steps)
	if err != nil {
		return nil, err
	}
	t, err := workflowtemplate.New(cmd.Kind, cmd.Name, cmd.Description, steps, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// GetQuery loads a template by id.
type GetQuery struct{ ID uuid.UUID }

// GetHandler loads a template.
type GetHandler struct{ repo workflowtemplate.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r workflowtemplate.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle executes the get.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*workflowtemplate.Template, error) {
	return h.repo.GetByID(ctx, q.ID)
}

// UpdateCommand creates a NEW version of an existing template (kind inherited from prior).
type UpdateCommand struct {
	ID          uuid.UUID
	Name        string
	Description string
	Steps       []StepInput
	UpdatedBy   string
}

// UpdateHandler creates a new template version (inactive).
type UpdateHandler struct{ repo workflowtemplate.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r workflowtemplate.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle creates a new version row referencing the prior kind.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*workflowtemplate.Template, error) {
	prior, err := h.repo.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	steps, err := buildSteps(cmd.Steps)
	if err != nil {
		return nil, err
	}
	nextVersion, err := workflowtemplate.NewVersion(prior, cmd.Name, cmd.Description, steps, cmd.UpdatedBy)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, nextVersion); err != nil {
		return nil, err
	}
	return nextVersion, nil
}

// ActivateCommand activates a specific version.
type ActivateCommand struct {
	ID uuid.UUID
	By string
}

// ActivateHandler activates the given template (deactivating sibling versions).
type ActivateHandler struct{ repo workflowtemplate.Repository }

// NewActivateHandler constructs an ActivateHandler.
func NewActivateHandler(r workflowtemplate.Repository) *ActivateHandler {
	return &ActivateHandler{repo: r}
}

// Handle executes activation.
func (h *ActivateHandler) Handle(ctx context.Context, cmd ActivateCommand) (*workflowtemplate.Template, error) {
	return h.repo.Activate(ctx, cmd.ID, cmd.By)
}

// DeleteCommand identifies the row to soft-delete.
type DeleteCommand struct {
	ID        uuid.UUID
	DeletedBy string
}

// DeleteHandler soft-deletes a template.
type DeleteHandler struct{ repo workflowtemplate.Repository }

// NewDeleteHandler constructs a DeleteHandler.
func NewDeleteHandler(r workflowtemplate.Repository) *DeleteHandler { return &DeleteHandler{repo: r} }

// Handle executes the soft delete.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	if err := h.repo.SoftDelete(ctx, cmd.ID, cmd.DeletedBy); err != nil {
		if errors.Is(err, workflowtemplate.ErrNotFound) {
			return err
		}
		return fmt.Errorf("delete workflow template: %w", err)
	}
	return nil
}

// ListQuery is the list params.
type ListQuery struct {
	Search       string
	Kind         string
	ActiveFilter string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

// ListResult bundles a page with total count.
type ListResult struct {
	Items []*workflowtemplate.Template
	Total int64
}

// ListHandler returns a page of templates.
type ListHandler struct{ repo workflowtemplate.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r workflowtemplate.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle executes the list.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, workflowtemplate.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}

// buildSteps validates per-step inputs and assembles domain Steps.
func buildSteps(in []StepInput) ([]workflowtemplate.Step, error) {
	out := make([]workflowtemplate.Step, 0, len(in))
	for _, s := range in {
		domStep, err := workflowtemplate.NewStep(
			s.StepNo, s.StepName, s.ResolutionType, s.ResolutionValue,
			s.SLAHours, s.AllowReject, s.AllowReassign, s.RequirePasswordOnUnlock,
			s.RejectToStepNo,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, domStep)
	}
	return out, nil
}
