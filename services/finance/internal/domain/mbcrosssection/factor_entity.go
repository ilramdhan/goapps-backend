package mbcrosssection

// Operation values for FactorEntity, mirroring the mbcf_operation CHECK in
// migration 000480. The operation carries the direction of the arithmetic and is
// not derivable from the factor alone.
const (
	// OperationMultiply means LDR_target = LDR_source * factor.
	OperationMultiply = "MULTIPLY"
	// OperationDivide means LDR_target = LDR_source / factor.
	OperationDivide = "DIVIDE"
)

// FactorEntity is one ORDERED (from_code -> to_code) LDR conversion factor
// (mst_mb_cross_section_factor).
type FactorEntity struct {
	id        string
	fromCode  string
	toCode    string
	factor    float64
	operation string
	note      string
	isActive  bool
	createdAt string
	createdBy string
	updatedAt string
	updatedBy string
	deletedAt string
	deletedBy string
}

// NewFactorEntity constructs a new conversion factor, enforcing the same invariants the
// table enforces: both codes present and within the column width, the pair not self-directed,
// a strictly positive factor and a known operation.
func NewFactorEntity(fromCode, toCode string, factor float64, operation, note string, isActive bool, createdBy string) (*FactorEntity, error) {
	if fromCode == "" || toCode == "" {
		return nil, ErrFactorCodeRequired
	}
	if len(fromCode) > MaxCodeLen || len(toCode) > MaxCodeLen {
		return nil, ErrCodeTooLong
	}
	if fromCode == toCode {
		return nil, ErrFactorSelfPair
	}
	if factor <= 0 {
		return nil, ErrFactorNotPositive
	}
	if !isValidOperation(operation) {
		return nil, ErrFactorInvalidOperation
	}
	if createdBy == "" {
		return nil, ErrCreatedByRequired
	}
	return &FactorEntity{
		fromCode:  fromCode,
		toCode:    toCode,
		factor:    factor,
		operation: operation,
		note:      note,
		isActive:  isActive,
		createdBy: createdBy,
	}, nil
}

func isValidOperation(op string) bool {
	return op == OperationMultiply || op == OperationDivide
}

// ReconstructFactor rebuilds a FactorEntity from storage without re-running construction validation.
//
//nolint:revive // Many parameters required for hydration from storage.
func ReconstructFactor(id, fromCode, toCode string, factor float64, operation, note string, isActive bool, createdAt, createdBy, updatedAt, updatedBy, deletedAt, deletedBy string) *FactorEntity {
	return &FactorEntity{
		id:        id,
		fromCode:  fromCode,
		toCode:    toCode,
		factor:    factor,
		operation: operation,
		note:      note,
		isActive:  isActive,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: updatedAt,
		updatedBy: updatedBy,
		deletedAt: deletedAt,
		deletedBy: deletedBy,
	}
}

// ID returns the factor row's UUID.
func (e *FactorEntity) ID() string { return e.id }

// FromCode returns the source cross-section code.
func (e *FactorEntity) FromCode() string { return e.fromCode }

// ToCode returns the target cross-section code.
func (e *FactorEntity) ToCode() string { return e.toCode }

// Factor returns the conversion factor.
func (e *FactorEntity) Factor() float64 { return e.factor }

// Operation returns the arithmetic direction (MULTIPLY or DIVIDE).
func (e *FactorEntity) Operation() string { return e.operation }

// Note returns the free-text note.
func (e *FactorEntity) Note() string { return e.note }

// IsActive returns whether the factor is active.
func (e *FactorEntity) IsActive() bool { return e.isActive }

// CreatedAt returns the creation timestamp.
func (e *FactorEntity) CreatedAt() string { return e.createdAt }

// CreatedBy returns the creator's identifier.
func (e *FactorEntity) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *FactorEntity) UpdatedAt() string { return e.updatedAt }

// UpdatedBy returns the last updater's identifier.
func (e *FactorEntity) UpdatedBy() string { return e.updatedBy }

// DeletedAt returns the soft-delete timestamp.
func (e *FactorEntity) DeletedAt() string { return e.deletedAt }

// DeletedBy returns the soft-deleter's identifier.
func (e *FactorEntity) DeletedBy() string { return e.deletedBy }

// IsDeleted returns whether the factor row has been soft-deleted.
func (e *FactorEntity) IsDeleted() bool { return e.deletedAt != "" }

// Update applies editable fields. The (from_code, to_code) pair is immutable: it is the
// row's business identity, guarded by the uix_mbcf_pair partial unique index. Changing a
// direction means deleting the row and creating the opposite one.
func (e *FactorEntity) Update(factor float64, operation, note string, isActive bool, updatedBy string) error {
	if e.IsDeleted() {
		return ErrDeleted
	}
	if updatedBy == "" {
		return ErrUpdatedByRequired
	}
	if factor <= 0 {
		return ErrFactorNotPositive
	}
	if !isValidOperation(operation) {
		return ErrFactorInvalidOperation
	}
	e.factor = factor
	e.operation = operation
	e.note = note
	e.isActive = isActive
	e.updatedBy = updatedBy
	return nil
}
