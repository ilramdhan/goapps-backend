// Package formula provides domain logic for Formula management.
package formula

import (
	"time"

	"github.com/google/uuid"
)

// Param represents an input parameter within a formula.
type Param struct {
	id        uuid.UUID
	paramID   uuid.UUID
	paramCode string
	paramName string
	sortOrder int
}

// NewParam creates a new Param.
func NewParam(paramID uuid.UUID, sortOrder int) *Param {
	return &Param{
		id:        uuid.New(),
		paramID:   paramID,
		sortOrder: sortOrder,
	}
}

// ReconstructParam reconstructs a Param from persistence.
func ReconstructParam(id, paramID uuid.UUID, paramCode, paramName string, sortOrder int) *Param {
	return &Param{
		id:        id,
		paramID:   paramID,
		paramCode: paramCode,
		paramName: paramName,
		sortOrder: sortOrder,
	}
}

// ID returns the formula param ID.
func (fp *Param) ID() uuid.UUID { return fp.id }

// ParamID returns the parameter reference ID.
func (fp *Param) ParamID() uuid.UUID { return fp.paramID }

// ParamCode returns the resolved parameter code.
func (fp *Param) ParamCode() string { return fp.paramCode }

// ParamName returns the resolved parameter name.
func (fp *Param) ParamName() string { return fp.paramName }

// SortOrder returns the display order.
func (fp *Param) SortOrder() int { return fp.sortOrder }

// =============================================================================
// Formula Aggregate Root
// =============================================================================

// Formula is the aggregate root for Formula domain.
type Formula struct {
	id              uuid.UUID
	code            Code
	name            string
	formulaType     Type
	expression      string
	resultParamID   uuid.UUID
	resultParamCode string
	resultParamName string
	description     string
	version         int
	isActive        bool
	inputParams     []*Param
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
	deletedAt       *time.Time
	deletedBy       *string
}

// NewFormula creates a new Formula entity with validation.
func NewFormula(
	code Code,
	name string,
	formulaType Type,
	expression string,
	resultParamID uuid.UUID,
	inputParamIDs []uuid.UUID,
	description string,
	createdBy string,
) (*Formula, error) {
	if err := validateFormulaFields(name, expression, description, createdBy); err != nil {
		return nil, err
	}

	if err := validateInputParams(resultParamID, inputParamIDs); err != nil {
		return nil, err
	}

	params := make([]*Param, len(inputParamIDs))
	for i, pid := range inputParamIDs {
		params[i] = NewParam(pid, i+1)
	}

	return &Formula{
		id:            uuid.New(),
		code:          code,
		name:          name,
		formulaType:   formulaType,
		expression:    expression,
		resultParamID: resultParamID,
		description:   description,
		version:       1,
		isActive:      true,
		inputParams:   params,
		createdAt:     time.Now(),
		createdBy:     createdBy,
	}, nil
}

func validateFormulaFields(name, expression, description, createdBy string) error {
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > 200 {
		return ErrNameTooLong
	}
	if expression == "" {
		return ErrEmptyExpression
	}
	if len(expression) > 5000 {
		return ErrExpressionTooLong
	}
	if len(description) > 1000 {
		return ErrDescriptionTooLong
	}
	if createdBy == "" {
		return ErrEmptyCreatedBy
	}
	return nil
}

func validateInputParams(resultParamID uuid.UUID, inputParamIDs []uuid.UUID) error {
	seen := make(map[uuid.UUID]struct{})
	for _, pid := range inputParamIDs {
		if pid == resultParamID {
			return ErrCircularReference
		}
		if _, ok := seen[pid]; ok {
			return ErrDuplicateInputParam
		}
		seen[pid] = struct{}{}
	}
	return nil
}

// ReconstructFormula reconstructs a Formula entity from persistence data.
func ReconstructFormula(
	id uuid.UUID,
	code Code,
	name string,
	formulaType Type,
	expression string,
	resultParamID uuid.UUID,
	resultParamCode string,
	resultParamName string,
	description string,
	version int,
	isActive bool,
	inputParams []*Param,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
	deletedAt *time.Time,
	deletedBy *string,
) *Formula {
	return &Formula{
		id:              id,
		code:            code,
		name:            name,
		formulaType:     formulaType,
		expression:      expression,
		resultParamID:   resultParamID,
		resultParamCode: resultParamCode,
		resultParamName: resultParamName,
		description:     description,
		version:         version,
		isActive:        isActive,
		inputParams:     inputParams,
		createdAt:       createdAt,
		createdBy:       createdBy,
		updatedAt:       updatedAt,
		updatedBy:       updatedBy,
		deletedAt:       deletedAt,
		deletedBy:       deletedBy,
	}
}

// =============================================================================
// Getters
// =============================================================================

// ID returns the unique identifier.
func (f *Formula) ID() uuid.UUID { return f.id }

// Code returns the formula code.
func (f *Formula) Code() Code { return f.code }

// Name returns the display name.
func (f *Formula) Name() string { return f.name }

// FormulaType returns the formula type.
func (f *Formula) FormulaType() Type { return f.formulaType }

// Expression returns the expression.
func (f *Formula) Expression() string { return f.expression }

// ResultParamID returns the result parameter ID.
func (f *Formula) ResultParamID() uuid.UUID { return f.resultParamID }

// ResultParamCode returns the resolved result parameter code.
func (f *Formula) ResultParamCode() string { return f.resultParamCode }

// ResultParamName returns the resolved result parameter name.
func (f *Formula) ResultParamName() string { return f.resultParamName }

// Description returns the description.
func (f *Formula) Description() string { return f.description }

// Version returns the version number.
func (f *Formula) Version() int { return f.version }

// IsActive returns whether the formula is active.
func (f *Formula) IsActive() bool { return f.isActive }

// InputParams returns the input parameters.
func (f *Formula) InputParams() []*Param { return f.inputParams }

// CreatedAt returns the creation timestamp.
func (f *Formula) CreatedAt() time.Time { return f.createdAt }

// CreatedBy returns the creator.
func (f *Formula) CreatedBy() string { return f.createdBy }

// UpdatedAt returns the last update timestamp.
func (f *Formula) UpdatedAt() *time.Time { return f.updatedAt }

// UpdatedBy returns the last updater.
func (f *Formula) UpdatedBy() *string { return f.updatedBy }

// DeletedAt returns the soft delete timestamp.
func (f *Formula) DeletedAt() *time.Time { return f.deletedAt }

// DeletedBy returns who deleted the record.
func (f *Formula) DeletedBy() *string { return f.deletedBy }

// IsDeleted returns true if the formula is soft deleted.
func (f *Formula) IsDeleted() bool { return f.deletedAt != nil }

// =============================================================================
// Domain Behavior Methods
// =============================================================================

// Update updates the Formula with new values.
func (f *Formula) Update(
	name *string,
	formulaType *Type,
	expression *string,
	resultParamID *uuid.UUID,
	inputParamIDs []uuid.UUID,
	description *string,
	isActive *bool,
	updatedBy string,
) error {
	if f.IsDeleted() {
		return ErrAlreadyDeleted
	}

	if err := f.updateName(name); err != nil {
		return err
	}
	if err := f.updateFormulaType(formulaType); err != nil {
		return err
	}
	if err := f.updateExpression(expression); err != nil {
		return err
	}
	if err := f.updateDescription(description); err != nil {
		return err
	}

	if resultParamID != nil {
		f.resultParamID = *resultParamID
	}

	if inputParamIDs != nil {
		if err := validateInputParams(f.resultParamID, inputParamIDs); err != nil {
			return err
		}
		params := make([]*Param, len(inputParamIDs))
		for i, pid := range inputParamIDs {
			params[i] = NewParam(pid, i+1)
		}
		f.inputParams = params
	}

	if isActive != nil {
		f.isActive = *isActive
	}

	f.version++
	now := time.Now()
	f.updatedAt = &now
	f.updatedBy = &updatedBy

	return nil
}

func (f *Formula) updateName(name *string) error {
	if name == nil {
		return nil
	}
	if *name == "" {
		return ErrEmptyName
	}
	if len(*name) > 200 {
		return ErrNameTooLong
	}
	f.name = *name
	return nil
}

func (f *Formula) updateFormulaType(ft *Type) error {
	if ft == nil {
		return nil
	}
	if !ft.IsValid() {
		return ErrInvalidFormulaType
	}
	f.formulaType = *ft
	return nil
}

func (f *Formula) updateExpression(expression *string) error {
	if expression == nil {
		return nil
	}
	if *expression == "" {
		return ErrEmptyExpression
	}
	if len(*expression) > 5000 {
		return ErrExpressionTooLong
	}
	f.expression = *expression
	return nil
}

func (f *Formula) updateDescription(description *string) error {
	if description == nil {
		return nil
	}
	if len(*description) > 1000 {
		return ErrDescriptionTooLong
	}
	f.description = *description
	return nil
}

// SoftDelete marks the formula as deleted.
func (f *Formula) SoftDelete(deletedBy string) error {
	if f.IsDeleted() {
		return ErrAlreadyDeleted
	}

	now := time.Now()
	f.deletedAt = &now
	f.deletedBy = &deletedBy
	f.isActive = false

	return nil
}
