// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// UpdateCommand represents the update Formula command.
type UpdateCommand struct {
	FormulaID     string
	FormulaName   *string
	FormulaType   *string
	Expression    *string
	ResultParamID *string
	InputParamIDs []string // nil=no change, non-nil=replace all
	Description   *string
	IsActive      *bool
	UpdatedBy     string
}

// UpdateHandler handles the UpdateFormula command.
type UpdateHandler struct {
	repo formula.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo formula.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Formula command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*formula.Formula, error) {
	id, err := uuid.Parse(cmd.FormulaID)
	if err != nil {
		return nil, formula.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	formulaType, _, err := h.parseFormulaType(cmd.FormulaType)
	if err != nil {
		return nil, err
	}

	resultParamID, _, err := h.validateResultParam(ctx, cmd.ResultParamID, entity.ID())
	if err != nil {
		return nil, err
	}

	inputParamIDs, err := h.validateInputParams(ctx, cmd.InputParamIDs)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(
		cmd.FormulaName, formulaType, cmd.Expression,
		resultParamID, inputParamIDs, cmd.Description,
		cmd.IsActive, cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return h.repo.GetByID(ctx, entity.ID())
}

// parseFormulaType parses an optional FormulaType string.
// Returns (nil, false, nil) when ft is nil (no change requested).
func (h *UpdateHandler) parseFormulaType(ft *string) (*formula.Type, bool, error) {
	if ft == nil {
		return nil, false, nil
	}
	parsed, err := formula.NewType(*ft)
	if err != nil {
		return nil, false, err
	}
	return &parsed, true, nil
}

// validateResultParam validates and parses the optional result parameter ID.
// Returns (nil, false, nil) when resultParamIDStr is nil (no change requested).
func (h *UpdateHandler) validateResultParam(ctx context.Context, resultParamIDStr *string, entityID uuid.UUID) (*uuid.UUID, bool, error) {
	if resultParamIDStr == nil {
		return nil, false, nil
	}

	rpID, err := uuid.Parse(*resultParamIDStr)
	if err != nil {
		return nil, false, formula.ErrResultParamNotFound
	}

	exists, err := h.repo.ParamExistsByID(ctx, rpID)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, formula.ErrResultParamNotFound
	}

	used, err := h.repo.ResultParamUsedByOther(ctx, rpID, entityID)
	if err != nil {
		return nil, false, err
	}
	if used {
		return nil, false, formula.ErrResultParamAlreadyUsed
	}

	return &rpID, true, nil
}

// validateInputParams validates and parses input parameter IDs.
func (h *UpdateHandler) validateInputParams(ctx context.Context, inputParamIDStrs []string) ([]uuid.UUID, error) {
	if inputParamIDStrs == nil {
		return nil, nil
	}

	inputParamIDs := make([]uuid.UUID, 0, len(inputParamIDStrs))
	for _, idStr := range inputParamIDStrs {
		paramID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, formula.ErrInputParamNotFound
		}
		exists, err := h.repo.ParamExistsByID(ctx, paramID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, formula.ErrInputParamNotFound
		}
		inputParamIDs = append(inputParamIDs, paramID)
	}

	return inputParamIDs, nil
}
