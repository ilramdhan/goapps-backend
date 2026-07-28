// Package lookup provides domain logic for the PPC lookup master. The lookup
// master is DISPLAY metadata only: it supplies human-friendly dropdown labels
// for enum codes. Backend business logic keeps validating against Go enum
// constants — this table never drives a business decision.
package lookup

import "time"

const (
	maxCategoryLen = 40
	maxCodeLen     = 40
	maxLabelLen    = 120
)

// Lookup is the aggregate root for a PPC lookup row.
type Lookup struct {
	id        int64
	category  string
	code      string
	label     string
	sortOrder int32
	isActive  bool
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
}

// NewLookup creates a new Lookup with validation.
func NewLookup(category, code, label string, sortOrder int32, createdBy string) (*Lookup, error) {
	if err := validateCategory(category); err != nil {
		return nil, err
	}
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := validateLabel(label); err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Lookup{
		category:  category,
		code:      code,
		label:     label,
		sortOrder: sortOrder,
		isActive:  true,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// Reconstruct rebuilds a Lookup from persistence (no validation).
func Reconstruct(
	id int64,
	category, code, label string,
	sortOrder int32,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Lookup {
	return &Lookup{
		id:        id,
		category:  category,
		code:      code,
		label:     label,
		sortOrder: sortOrder,
		isActive:  isActive,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: updatedAt,
		updatedBy: updatedBy,
	}
}

// ID returns the lookup identifier.
func (l *Lookup) ID() int64 { return l.id }

// Category returns the lookup category.
func (l *Lookup) Category() string { return l.category }

// Code returns the lookup code (equals the enum string value).
func (l *Lookup) Code() string { return l.code }

// Label returns the human-friendly label.
func (l *Lookup) Label() string { return l.label }

// SortOrder returns the sort order.
func (l *Lookup) SortOrder() int32 { return l.sortOrder }

// IsActive returns whether the lookup is active.
func (l *Lookup) IsActive() bool { return l.isActive }

// CreatedAt returns the creation timestamp.
func (l *Lookup) CreatedAt() time.Time { return l.createdAt }

// CreatedBy returns the creator.
func (l *Lookup) CreatedBy() string { return l.createdBy }

// UpdatedAt returns the last update timestamp.
func (l *Lookup) UpdatedAt() *time.Time { return l.updatedAt }

// UpdatedBy returns the last updater.
func (l *Lookup) UpdatedBy() *string { return l.updatedBy }

// Update applies optional field changes with validation. Category and code are
// immutable (they pin the lookup to an enum value).
func (l *Lookup) Update(label *string, sortOrder *int32, isActive *bool, updatedBy string) error {
	if label != nil {
		if err := validateLabel(*label); err != nil {
			return err
		}
		l.label = *label
	}
	if sortOrder != nil {
		l.sortOrder = *sortOrder
	}
	if isActive != nil {
		l.isActive = *isActive
	}

	now := time.Now()
	l.updatedAt = &now
	l.updatedBy = &updatedBy
	return nil
}

func validateCategory(category string) error {
	if category == "" {
		return ErrEmptyCategory
	}
	if len(category) > maxCategoryLen {
		return ErrCategoryTooLong
	}
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

func validateLabel(label string) error {
	if label == "" {
		return ErrEmptyLabel
	}
	if len(label) > maxLabelLen {
		return ErrLabelTooLong
	}
	return nil
}
