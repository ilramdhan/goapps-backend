// Package employeelevel provides application layer handlers for Employee Level operations.
package employeelevel

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/employeelevel"
	"github.com/mutugading/goapps-backend/services/shared/workflow"
)

const entityType = "employee_level"

// employeeLevelWorkflowEngine defines the allowed transitions.
var employeeLevelWorkflowEngine = workflow.NewEngine([]workflow.Transition{
	{From: workflow.State(employeelevel.WorkflowDraft), To: workflow.State(employeelevel.WorkflowSubmitted), Action: workflow.ActionSubmit},
	{From: workflow.State(employeelevel.WorkflowSubmitted), To: workflow.State(employeelevel.WorkflowApproved), Action: workflow.ActionApprove},
	{From: workflow.State(employeelevel.WorkflowApproved), To: workflow.State(employeelevel.WorkflowReleased), Action: workflow.ActionRelease},
})

// WorkflowCommand is the input for any workflow transition.
type WorkflowCommand struct {
	EmployeeLevelID string
	Notes           string
	UserID          string
}

// WorkflowHandler handles workflow state transitions.
type WorkflowHandler struct {
	repo        employeelevel.Repository
	historyRepo employeelevel.WorkflowHistoryRepository
}

// NewWorkflowHandler creates a new WorkflowHandler.
func NewWorkflowHandler(repo employeelevel.Repository, historyRepo employeelevel.WorkflowHistoryRepository) *WorkflowHandler {
	return &WorkflowHandler{repo: repo, historyRepo: historyRepo}
}

// Submit transitions DRAFT → SUBMITTED.
func (h *WorkflowHandler) Submit(ctx context.Context, cmd WorkflowCommand) (*employeelevel.EmployeeLevel, error) {
	return h.transition(ctx, cmd, workflow.ActionSubmit)
}

// Approve transitions SUBMITTED → APPROVED.
func (h *WorkflowHandler) Approve(ctx context.Context, cmd WorkflowCommand) (*employeelevel.EmployeeLevel, error) {
	return h.transition(ctx, cmd, workflow.ActionApprove)
}

// Release transitions APPROVED → RELEASED.
func (h *WorkflowHandler) Release(ctx context.Context, cmd WorkflowCommand) (*employeelevel.EmployeeLevel, error) {
	return h.transition(ctx, cmd, workflow.ActionRelease)
}

// BypassRelease transitions from any pre-release state directly to RELEASED.
func (h *WorkflowHandler) BypassRelease(ctx context.Context, cmd WorkflowCommand) (*employeelevel.EmployeeLevel, error) {
	entity, err := h.getEntity(ctx, cmd.EmployeeLevelID)
	if err != nil {
		return nil, err
	}

	current := workflow.State(entity.Workflow())
	target := workflow.State(employeelevel.WorkflowReleased)
	if err := employeeLevelWorkflowEngine.ValidateBypass(current, target); err != nil {
		return nil, err
	}

	fromState := int32(entity.Workflow())
	entity.SetWorkflow(employeelevel.WorkflowReleased, cmd.UserID)

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	h.recordHistory(ctx, entity.ID(), fromState, int32(employeelevel.WorkflowReleased), string(workflow.ActionBypassRelease), cmd.UserID, cmd.Notes)
	return entity, nil
}

func (h *WorkflowHandler) transition(ctx context.Context, cmd WorkflowCommand, action workflow.Action) (*employeelevel.EmployeeLevel, error) {
	entity, err := h.getEntity(ctx, cmd.EmployeeLevelID)
	if err != nil {
		return nil, err
	}

	current := workflow.State(entity.Workflow())
	target, err := employeeLevelWorkflowEngine.Validate(current, action)
	if err != nil {
		return nil, err
	}

	fromState := int32(entity.Workflow())
	entity.SetWorkflow(employeelevel.Workflow(target), cmd.UserID)

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	h.recordHistory(ctx, entity.ID(), fromState, int32(target), string(action), cmd.UserID, cmd.Notes)
	return entity, nil
}

func (h *WorkflowHandler) getEntity(ctx context.Context, id string) (*employeelevel.EmployeeLevel, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid employee level ID: %w", err)
	}
	entity, err := h.repo.GetByID(ctx, parsed)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (h *WorkflowHandler) recordHistory(ctx context.Context, entityID uuid.UUID, from, to int32, action, userID, notes string) {
	if h.historyRepo == nil {
		return
	}
	entry := &employeelevel.WorkflowHistory{
		ID:         uuid.New(),
		EntityType: entityType,
		EntityID:   entityID,
		FromState:  from,
		ToState:    to,
		Action:     action,
		UserID:     userID,
		Notes:      notes,
	}
	if err := h.historyRepo.Record(ctx, entry); err != nil {
		// Non-critical — log but don't fail the transition.
		fmt.Printf("warning: failed to record workflow history: %v\n", err)
	}
}
