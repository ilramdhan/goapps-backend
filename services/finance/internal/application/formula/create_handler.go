// Package formula provides application layer handlers for Formula operations.
package formula

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// CreateCommand represents the create Formula command.
type CreateCommand struct {
	FormulaCode   string
	FormulaName   string
	FormulaType   string
	Expression    string
	ResultParamID string
	InputParamIDs []string
	Description   string
	CreatedBy     string
}

// CreateHandler handles the CreateFormula command.
type CreateHandler struct {
	repo formula.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo formula.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create Formula command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*formula.Formula, error) {
	// 1. Validate value objects
	code, err := formula.NewCode(cmd.FormulaCode)
	if err != nil {
		return nil, err
	}

	formulaType, err := formula.NewType(cmd.FormulaType)
	if err != nil {
		return nil, err
	}

	// 2. Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, formula.ErrAlreadyExists
	}

	// 3. Parse and validate result param
	resultParamID, err := uuid.Parse(cmd.ResultParamID)
	if err != nil {
		return nil, formula.ErrResultParamNotFound
	}

	paramExists, err := h.repo.ParamExistsByID(ctx, resultParamID)
	if err != nil {
		return nil, err
	}
	if !paramExists {
		return nil, formula.ErrResultParamNotFound
	}

	// 4. Check result param not used by another formula
	used, err := h.repo.ResultParamUsedByOther(ctx, resultParamID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	if used {
		return nil, formula.ErrResultParamAlreadyUsed
	}

	// 5. Parse and validate input params
	inputParamIDs, err := h.parseAndValidateInputParams(ctx, cmd.InputParamIDs)
	if err != nil {
		return nil, err
	}

	// 6. Create domain entity
	entity, err := formula.NewFormula(
		code, cmd.FormulaName, formulaType,
		cmd.Expression, resultParamID, inputParamIDs,
		cmd.Description, cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	// 7. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	// 8. Return full entity with joins
	return h.repo.GetByID(ctx, entity.ID())
}

func (h *CreateHandler) parseAndValidateInputParams(ctx context.Context, ids []string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, 0, len(ids))
	for _, idStr := range ids {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, formula.ErrInputParamNotFound
		}
		exists, err := h.repo.ParamExistsByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, formula.ErrInputParamNotFound
		}
		result = append(result, id)
	}
	return result, nil
}
