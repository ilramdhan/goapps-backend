// Package parameter provides application layer handlers for Parameter operations.
package parameter

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// UpdateCommand represents the update Parameter command.
type UpdateCommand struct {
	ParamID        string
	ParamName      *string
	ParamShortName *string
	DataType       *string
	ParamCategory  *string
	UOMID          *string // nil=not set, pointer to ""=clear, pointer to uuid=set
	DefaultValue   *string // nil=not set, pointer to ""=clear, pointer to value=set
	MinValue       *string // nil=not set, pointer to ""=clear, pointer to value=set
	MaxValue       *string // nil=not set, pointer to ""=clear, pointer to value=set
	IsActive       *bool
	UpdatedBy      string
}

// UpdateHandler handles the UpdateParameter command.
type UpdateHandler struct {
	repo parameter.Repository
}

// NewUpdateHandler creates a new UpdateHandler.
func NewUpdateHandler(repo parameter.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the update Parameter command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*parameter.Parameter, error) {
	id, err := uuid.Parse(cmd.ParamID)
	if err != nil {
		return nil, parameter.ErrNotFound
	}

	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	dataType, err := parseOptionalDataType(cmd.DataType)
	if err != nil {
		return nil, err
	}

	paramCategory, err := parseOptionalParamCategory(cmd.ParamCategory)
	if err != nil {
		return nil, err
	}

	uomID, err := parseOptionalUOMID(cmd.UOMID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(
		cmd.ParamName, cmd.ParamShortName,
		dataType, paramCategory, uomID,
		toDoublePointer(cmd.DefaultValue),
		toDoublePointer(cmd.MinValue),
		toDoublePointer(cmd.MaxValue),
		cmd.IsActive, cmd.UpdatedBy,
	); err != nil {
		return nil, err
	}

	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}

	return entity, nil
}

func parseOptionalDataType(dt *string) (*parameter.DataType, error) {
	if dt == nil {
		return nil, nil //nolint:nilnil // nil means "not set" — intentional sentinel-free design
	}
	parsed, err := parameter.NewDataType(*dt)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalParamCategory(cat *string) (*parameter.ParamCategory, error) {
	if cat == nil {
		return nil, nil //nolint:nilnil // nil means "not set" — intentional sentinel-free design
	}
	parsed, err := parameter.NewParamCategory(*cat)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalUOMID(uomID *string) (**uuid.UUID, error) {
	if uomID == nil {
		return nil, nil //nolint:nilnil // nil means "not set" — intentional sentinel-free design
	}
	if *uomID == "" {
		var nilUOM *uuid.UUID
		return &nilUOM, nil
	}
	parsed, err := uuid.Parse(*uomID)
	if err != nil {
		return nil, parameter.ErrNotFound
	}
	parsedPtr := &parsed
	return &parsedPtr, nil
}

// toDoublePointer converts *string to **string for the domain Update method.
// nil=not set, *nil=clear, *value=set.
func toDoublePointer(val *string) **string {
	if val == nil {
		return nil
	}
	if *val == "" {
		var nilStr *string
		return &nilStr
	}
	return &val
}
