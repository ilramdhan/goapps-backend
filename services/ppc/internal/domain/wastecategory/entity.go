// Package wastecategory provides domain logic for PPC waste-category master data.
package wastecategory

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

const (
	maxCodeLen = 30
	maxNameLen = 100

	// TypeWaste is the WASTE waste-category type.
	TypeWaste = "WASTE"
	// TypeDowngrade is the DOWNGRADE waste-category type.
	TypeDowngrade = "DOWNGRADE"
)

// IsValidType reports whether s is one of the allowed waste-category types.
func IsValidType(s string) bool {
	return s == TypeWaste || s == TypeDowngrade
}

// Category is the aggregate root for the waste-category master.
type Category struct {
	id           int64
	categoryArea area.Area
	wasteType    string
	code         string
	name         string
	gradeTarget  *string
	isActive     bool
	sortOrder    int32
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
}

// NewCategory creates a new Category with validation.
func NewCategory(
	categoryArea area.Area,
	wasteType, code, name string,
	gradeTarget *string,
	sortOrder int32,
	createdBy string,
) (*Category, error) {
	if categoryArea.IsEmpty() {
		return nil, ErrInvalidArea
	}
	if !IsValidType(wasteType) {
		return nil, ErrInvalidType
	}
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	normalized, err := normalizeGradeTarget(wasteType, gradeTarget)
	if err != nil {
		return nil, err
	}

	return &Category{
		categoryArea: categoryArea,
		wasteType:    wasteType,
		code:         code,
		name:         name,
		gradeTarget:  normalized,
		isActive:     true,
		sortOrder:    sortOrder,
		createdAt:    time.Now(),
		createdBy:    createdBy,
	}, nil
}

// Reconstruct rebuilds a Category from persistence (no validation).
func Reconstruct(
	id int64,
	categoryArea area.Area,
	wasteType, code, name string,
	gradeTarget *string,
	isActive bool,
	sortOrder int32,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Category {
	return &Category{
		id:           id,
		categoryArea: categoryArea,
		wasteType:    wasteType,
		code:         code,
		name:         name,
		gradeTarget:  gradeTarget,
		isActive:     isActive,
		sortOrder:    sortOrder,
		createdAt:    createdAt,
		createdBy:    createdBy,
		updatedAt:    updatedAt,
		updatedBy:    updatedBy,
	}
}

// ID returns the category identifier.
func (c *Category) ID() int64 { return c.id }

// Area returns the category area.
func (c *Category) Area() area.Area { return c.categoryArea }

// Type returns the waste-category type (WASTE or DOWNGRADE).
func (c *Category) Type() string { return c.wasteType }

// Code returns the category code.
func (c *Category) Code() string { return c.code }

// Name returns the category name.
func (c *Category) Name() string { return c.name }

// GradeTarget returns the optional grade target.
func (c *Category) GradeTarget() *string { return c.gradeTarget }

// IsActive returns the active flag.
func (c *Category) IsActive() bool { return c.isActive }

// SortOrder returns the sort order.
func (c *Category) SortOrder() int32 { return c.sortOrder }

// CreatedAt returns the creation timestamp.
func (c *Category) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Category) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Category) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Category) UpdatedBy() *string { return c.updatedBy }

// Update applies optional field changes with validation.
func (c *Category) Update(name *string, gradeTarget *string, isActive *bool, sortOrder *int32, updatedBy string) error {
	if err := c.applyName(name); err != nil {
		return err
	}
	if err := c.applyGradeTarget(gradeTarget); err != nil {
		return err
	}
	if isActive != nil {
		c.isActive = *isActive
	}
	if sortOrder != nil {
		c.sortOrder = *sortOrder
	}

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy
	return nil
}

func (c *Category) applyName(name *string) error {
	if name == nil {
		return nil
	}
	if err := validateName(*name); err != nil {
		return err
	}
	c.name = *name
	return nil
}

func (c *Category) applyGradeTarget(gradeTarget *string) error {
	if gradeTarget == nil {
		return nil
	}
	normalized, err := normalizeGradeTarget(c.wasteType, gradeTarget)
	if err != nil {
		return err
	}
	c.gradeTarget = normalized
	return nil
}

// normalizeGradeTarget enforces the type/grade-target relationship: DOWNGRADE
// requires a non-empty grade target, WASTE always yields an empty (nil) target.
func normalizeGradeTarget(wasteType string, gradeTarget *string) (*string, error) {
	if wasteType == TypeWaste {
		return nil, nil //nolint:nilnil // WASTE type legitimately has no grade target
	}
	if gradeTarget == nil || *gradeTarget == "" {
		return nil, ErrGradeTargetRequired
	}
	return gradeTarget, nil
}

func validateCode(code string) error {
	if code == "" {
		return ErrEmptyCode
	}
	if len(code) > maxCodeLen {
		return ErrCodeTooLong
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > maxNameLen {
		return ErrNameTooLong
	}
	return nil
}
