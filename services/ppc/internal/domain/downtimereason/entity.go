// Package downtimereason provides domain logic for PPC downtime-reason master data.
package downtimereason

import (
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
)

const (
	maxCodeLen = 20
	maxNameLen = 100
)

// Category values for downtime reasons.
const (
	CategoryIdlePosition   = "IDLE_POSITION"
	CategoryMachineDown    = "MACHINE_DOWN"
	CategoryProductionLoss = "PRODUCTION_LOSS"
)

// IsValidCategory reports whether the category is one of the allowed values.
func IsValidCategory(category string) bool {
	switch category {
	case CategoryIdlePosition, CategoryMachineDown, CategoryProductionLoss:
		return true
	default:
		return false
	}
}

// Reason is the aggregate root for the downtime-reason master.
type Reason struct {
	id               int64
	reasonArea       area.Area
	code             string
	name             string
	category         string
	isExcludeFromEff bool
	isActive         bool
	sortOrder        int32
	createdAt        time.Time
	createdBy        string
	updatedAt        *time.Time
	updatedBy        *string
}

// NewReason creates a new Reason with validation.
func NewReason(reasonArea area.Area, code, name, category string, isExcludeFromEff bool, sortOrder int32, createdBy string) (*Reason, error) {
	if reasonArea.IsEmpty() {
		return nil, ErrInvalidArea
	}
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if !IsValidCategory(category) {
		return nil, ErrInvalidCategory
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Reason{
		reasonArea:       reasonArea,
		code:             code,
		name:             name,
		category:         category,
		isExcludeFromEff: isExcludeFromEff,
		isActive:         true,
		sortOrder:        sortOrder,
		createdAt:        time.Now(),
		createdBy:        createdBy,
	}, nil
}

// Reconstruct rebuilds a Reason from persistence (no validation).
func Reconstruct(
	id int64,
	reasonArea area.Area,
	code, name, category string,
	isExcludeFromEff, isActive bool,
	sortOrder int32,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Reason {
	return &Reason{
		id:               id,
		reasonArea:       reasonArea,
		code:             code,
		name:             name,
		category:         category,
		isExcludeFromEff: isExcludeFromEff,
		isActive:         isActive,
		sortOrder:        sortOrder,
		createdAt:        createdAt,
		createdBy:        createdBy,
		updatedAt:        updatedAt,
		updatedBy:        updatedBy,
	}
}

// ID returns the reason identifier.
func (r *Reason) ID() int64 { return r.id }

// Area returns the reason area.
func (r *Reason) Area() area.Area { return r.reasonArea }

// Code returns the reason code.
func (r *Reason) Code() string { return r.code }

// Name returns the reason name.
func (r *Reason) Name() string { return r.name }

// Category returns the reason category.
func (r *Reason) Category() string { return r.category }

// IsExcludeFromEff returns whether the reason is excluded from efficiency.
func (r *Reason) IsExcludeFromEff() bool { return r.isExcludeFromEff }

// IsActive returns whether the reason is active.
func (r *Reason) IsActive() bool { return r.isActive }

// SortOrder returns the sort order.
func (r *Reason) SortOrder() int32 { return r.sortOrder }

// CreatedAt returns the creation timestamp.
func (r *Reason) CreatedAt() time.Time { return r.createdAt }

// CreatedBy returns the creator.
func (r *Reason) CreatedBy() string { return r.createdBy }

// UpdatedAt returns the last update timestamp.
func (r *Reason) UpdatedAt() *time.Time { return r.updatedAt }

// UpdatedBy returns the last updater.
func (r *Reason) UpdatedBy() *string { return r.updatedBy }

// Update applies optional field changes with validation (area and code are not mutable).
func (r *Reason) Update(name, category *string, isExcludeFromEff, isActive *bool, sortOrder *int32, updatedBy string) error {
	if err := r.applyName(name); err != nil {
		return err
	}
	if err := r.applyCategory(category); err != nil {
		return err
	}
	if isExcludeFromEff != nil {
		r.isExcludeFromEff = *isExcludeFromEff
	}
	if isActive != nil {
		r.isActive = *isActive
	}
	if sortOrder != nil {
		r.sortOrder = *sortOrder
	}

	now := time.Now()
	r.updatedAt = &now
	r.updatedBy = &updatedBy
	return nil
}

func (r *Reason) applyName(name *string) error {
	if name == nil {
		return nil
	}
	if err := validateName(*name); err != nil {
		return err
	}
	r.name = *name
	return nil
}

func (r *Reason) applyCategory(category *string) error {
	if category == nil {
		return nil
	}
	if !IsValidCategory(*category) {
		return ErrInvalidCategory
	}
	r.category = *category
	return nil
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
