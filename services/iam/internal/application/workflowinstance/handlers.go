// Package workflowinstance contains use cases for workflow instance lifecycle.
package workflowinstance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowinstance"
	domtpl "github.com/mutugading/goapps-backend/services/iam/internal/domain/workflowtemplate"
)

// StartCommand starts a new instance from a specific template.
type StartCommand struct {
	TemplateID uuid.UUID
	EntityKind string
	EntityID   uuid.UUID
	StartedBy  string
}

// StartHandler snapshots the template and creates an instance.
type StartHandler struct {
	tplRepo domtpl.Repository
	insRepo workflowinstance.Repository
}

// NewStartHandler constructs a StartHandler.
func NewStartHandler(tplRepo domtpl.Repository, insRepo workflowinstance.Repository) *StartHandler {
	return &StartHandler{tplRepo: tplRepo, insRepo: insRepo}
}

// Handle creates an instance using the requested template.
// The instance is wired to the supplied entity via (entity_kind, entity_id).
func (h *StartHandler) Handle(ctx context.Context, cmd StartCommand) (*workflowinstance.Instance, error) {
	tpl, err := h.tplRepo.GetByID(ctx, cmd.TemplateID)
	if err != nil {
		return nil, err
	}
	if !tpl.IsActive() {
		return nil, workflowinstance.ErrNoActiveTemplate
	}
	steps := tpl.Steps()
	if len(steps) == 0 {
		return nil, domtpl.ErrNoSteps
	}
	first := steps[0]
	ins, err := workflowinstance.New(
		tpl.ID(), tpl.Version(), tpl.Kind().String(), cmd.EntityKind, cmd.EntityID, cmd.StartedBy,
		first.StepName(), first.ApproverResolutionType().String(), first.ApproverResolutionValue(),
		first.SLAHours(), first.AllowReject(), first.RequirePasswordOnUnlock(),
		len(steps),
	)
	if err != nil {
		return nil, err
	}
	if err := h.insRepo.Create(ctx, ins); err != nil {
		return nil, err
	}
	return ins, nil
}

// GetQuery loads an instance.
type GetQuery struct{ ID uuid.UUID }

// GetHandler returns an instance with steps preloaded.
type GetHandler struct{ repo workflowinstance.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r workflowinstance.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle executes the get.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*workflowinstance.Instance, error) {
	return h.repo.GetByID(ctx, q.ID)
}

// AdvanceCommand approves the current step.
type AdvanceCommand struct {
	InstanceID uuid.UUID
	Actor      uuid.UUID
	Comment    string
}

// AdvanceHandler approves the current step and possibly locks the instance.
type AdvanceHandler struct {
	tplRepo domtpl.Repository
	insRepo workflowinstance.Repository
}

// NewAdvanceHandler constructs an AdvanceHandler.
func NewAdvanceHandler(tplRepo domtpl.Repository, insRepo workflowinstance.Repository) *AdvanceHandler {
	return &AdvanceHandler{tplRepo: tplRepo, insRepo: insRepo}
}

// Handle executes the advance.
func (h *AdvanceHandler) Handle(ctx context.Context, cmd AdvanceCommand) (*workflowinstance.Instance, error) {
	ins, err := h.insRepo.GetByID(ctx, cmd.InstanceID)
	if err != nil {
		return nil, err
	}
	// Build a next-step factory that reads from the template snapshot.
	tpl, err := h.tplRepo.GetByID(ctx, ins.TemplateID())
	if err != nil {
		return nil, fmt.Errorf("load template snapshot: %w", err)
	}
	nextFactory := func(stepNo int) (workflowinstance.Step, error) {
		for _, s := range tpl.Steps() {
			if s.StepNo() == stepNo {
				return workflowinstance.NewPendingStep(
					ins.ID(), stepNo, s.StepName(),
					s.ApproverResolutionType().String(), s.ApproverResolutionValue(),
					s.SLAHours(), s.AllowReject(), s.RequirePasswordOnUnlock(),
				), nil
			}
		}
		return workflowinstance.Step{}, workflowinstance.ErrCurrentStepMissing
	}
	if _, err := ins.Advance(cmd.Actor, cmd.Comment, nextFactory); err != nil {
		if errors.Is(err, workflowinstance.ErrNotInProgress) ||
			errors.Is(err, workflowinstance.ErrCurrentStepMissing) {
			return nil, err
		}
		return nil, fmt.Errorf("advance: %w", err)
	}
	if err := h.insRepo.SaveTransition(ctx, ins); err != nil {
		return nil, err
	}
	return ins, nil
}

// RejectCommand rejects the current step.
type RejectCommand struct {
	InstanceID uuid.UUID
	Actor      uuid.UUID
	Comment    string
}

// RejectHandler rejects an instance.
type RejectHandler struct{ repo workflowinstance.Repository }

// NewRejectHandler constructs a RejectHandler.
func NewRejectHandler(r workflowinstance.Repository) *RejectHandler {
	return &RejectHandler{repo: r}
}

// Handle executes the reject.
func (h *RejectHandler) Handle(ctx context.Context, cmd RejectCommand) (*workflowinstance.Instance, error) {
	ins, err := h.repo.GetByID(ctx, cmd.InstanceID)
	if err != nil {
		return nil, err
	}
	if err := ins.Reject(cmd.Actor, cmd.Comment); err != nil {
		return nil, err
	}
	if err := h.repo.SaveTransition(ctx, ins); err != nil {
		return nil, err
	}
	return ins, nil
}

// ListQuery is the list params.
type ListQuery struct {
	EntityKind string
	EntityID   string
	Status     string
	Page       int
	PageSize   int
}

// ListResult bundles items + total.
type ListResult struct {
	Items []*workflowinstance.Instance
	Total int64
}

// ListHandler returns a paginated set of instances (steps NOT preloaded).
type ListHandler struct{ repo workflowinstance.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r workflowinstance.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle executes the list.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, workflowinstance.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}
