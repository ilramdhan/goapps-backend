// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// CreateCommand represents the create Parameter command.
type CreateCommand struct {
	ParamCode      string
	ParamName      string
	ParamShortName string
	DataType       string
	ParamCategory  string
	UOMID          string // UUID string, empty means no UOM
	DefaultValue   string
	MinValue       string
	MaxValue       string
	CreatedBy      string
}

// CreateHandler handles the CreateParameter command.
type CreateHandler struct {
	repo parameter.Repository
}

// NewCreateHandler creates a new CreateHandler.
func NewCreateHandler(repo parameter.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the create Parameter command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*parameter.Parameter, error) {
	// 1. Validate and create value objects
	code, err := parameter.NewCode(cmd.ParamCode)
	if err != nil {
		return nil, err
	}

	dataType, err := parameter.NewDataType(cmd.DataType)
	if err != nil {
		return nil, err
	}

	paramCategory, err := parameter.NewParamCategory(cmd.ParamCategory)
	if err != nil {
		return nil, err
	}

	// 2. Check for duplicates
	exists, err := h.repo.ExistsByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, parameter.ErrAlreadyExists
	}

	// 3. Parse optional UOM ID
	var uomID *uuid.UUID
	if cmd.UOMID != "" {
		id, err := uuid.Parse(cmd.UOMID)
		if err != nil {
			return nil, parameter.ErrInvalidDataType // invalid UUID format
		}
		uomID = &id
	}

	// 4. Parse optional string fields
	var defaultValue, minValue, maxValue *string
	if cmd.DefaultValue != "" {
		defaultValue = &cmd.DefaultValue
	}
	if cmd.MinValue != "" {
		minValue = &cmd.MinValue
	}
	if cmd.MaxValue != "" {
		maxValue = &cmd.MaxValue
	}

	// 5. Create domain entity
	entity, err := parameter.NewParameter(
		code, cmd.ParamName, cmd.ParamShortName,
		dataType, paramCategory, uomID,
		defaultValue, minValue, maxValue,
		cmd.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	// 6. Persist
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}
