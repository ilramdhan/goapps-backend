// Package mbcrosssection provides domain logic for MB (Master Batch) cross-section master data.
package mbcrosssection

// Entity is a single cross-section master-data row (mst_mb_cross_section).
type Entity struct {
	id           string
	code         string
	displayName  string
	description  string
	isActive     bool
	displayOrder int32
	createdAt    string
	createdBy    string
	updatedAt    string
	updatedBy    string
	deletedAt    string
	deletedBy    string
}

// NewEntity constructs a new cross-section row, validating code and createdBy are present
// and defaulting isActive to true.
//
// The code is stored verbatim: every seeded value (RND, TBL, OTL, SPC, PLUS, RSD) is valid
// and no normalization or whitelisting is applied here — the master is user-editable.
func NewEntity(code, displayName, description string, displayOrder int32, createdBy string) (*Entity, error) {
	if code == "" {
		return nil, ErrCodeRequired
	}
	if len(code) > MaxCodeLen {
		return nil, ErrCodeTooLong
	}
	if createdBy == "" {
		return nil, ErrCreatedByRequired
	}
	return &Entity{
		code:         code,
		displayName:  displayName,
		description:  description,
		displayOrder: displayOrder,
		isActive:     true,
		createdBy:    createdBy,
	}, nil
}

// MaxCodeLen is the maximum length of mbcs_code (VARCHAR(10)).
const MaxCodeLen = 10

// Reconstruct rebuilds an Entity from storage without re-running construction validation.
//
//nolint:revive // Many parameters required for hydration from storage.
func Reconstruct(id, code, displayName, description string, displayOrder int32, isActive bool, createdAt, createdBy, updatedAt, updatedBy, deletedAt, deletedBy string) *Entity {
	return &Entity{
		id:           id,
		code:         code,
		displayName:  displayName,
		description:  description,
		displayOrder: displayOrder,
		isActive:     isActive,
		createdAt:    createdAt,
		createdBy:    createdBy,
		updatedAt:    updatedAt,
		updatedBy:    updatedBy,
		deletedAt:    deletedAt,
		deletedBy:    deletedBy,
	}
}

// ID returns the cross-section row's UUID.
func (e *Entity) ID() string { return e.id }

// Code returns the cross-section's business code.
func (e *Entity) Code() string { return e.code }

// DisplayName returns the cross-section's display name.
func (e *Entity) DisplayName() string { return e.displayName }

// Description returns the cross-section's description.
func (e *Entity) Description() string { return e.description }

// IsActive returns whether the cross-section is active.
func (e *Entity) IsActive() bool { return e.isActive }

// DisplayOrder returns the cross-section's display order.
func (e *Entity) DisplayOrder() int32 { return e.displayOrder }

// CreatedAt returns the creation timestamp.
func (e *Entity) CreatedAt() string { return e.createdAt }

// CreatedBy returns the creator's identifier.
func (e *Entity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entity) UpdatedAt() string { return e.updatedAt }

// UpdatedBy returns the last updater's identifier.
func (e *Entity) UpdatedBy() string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *Entity) DeletedAt() string { return e.deletedAt }

// DeletedBy returns the soft-deleter's identifier.
func (e *Entity) DeletedBy() string { return e.deletedBy }

// IsDeleted returns whether the cross-section row has been soft-deleted.
func (e *Entity) IsDeleted() bool { return e.deletedAt != "" }

// Update applies editable fields to an existing row. The code is immutable: it is
// referenced by mst_mb_cross_section_factor via a foreign key.
func (e *Entity) Update(displayName, description string, displayOrder int32, isActive bool, updatedBy string) error {
	if e.IsDeleted() {
		return ErrDeleted
	}
	if updatedBy == "" {
		return ErrUpdatedByRequired
	}
	e.displayName = displayName
	e.description = description
	e.displayOrder = displayOrder
	e.isActive = isActive
	e.updatedBy = updatedBy
	return nil
}

// Deactivate marks the cross-section inactive.
func (e *Entity) Deactivate() { e.isActive = false }
