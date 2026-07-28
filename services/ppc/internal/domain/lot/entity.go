// Package lot provides domain logic for PPC lot-master data.
package lot

import "time"

const (
	maxLotNoLen     = 30
	maxItemCodeLen  = 30
	maxShadeCodeLen = 20
)

// Master is the aggregate root for the lot-master.
type Master struct {
	lotNo           string
	itemCode        string
	shadeCode       string
	stdWeightFull   float64
	stdWeightUnfull float64
	notes           string
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string

	// source is SourcePPC or SourceMMSMERGE; see spec.go.
	source string
	// sourceKey is the source system's own key (MMSMERGE.MERGE_CODE).
	sourceKey string
	// syncedAt is when the sync last touched this row; nil for PPC-minted lots.
	syncedAt *time.Time
	// spec is the source-owned yarn/packing specification.
	spec Spec
}

// NewMaster creates a new Master with validation.
func NewMaster(lotNo, itemCode, shadeCode string, stdFull, stdUnfull float64, notes, createdBy string) (*Master, error) {
	if err := validateLotNo(lotNo); err != nil {
		return nil, err
	}
	if err := validateItemCode(itemCode); err != nil {
		return nil, err
	}
	if err := validateShadeCode(shadeCode); err != nil {
		return nil, err
	}
	if stdFull <= 0 || stdUnfull <= 0 {
		return nil, ErrInvalidWeight
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Master{
		lotNo:           lotNo,
		itemCode:        itemCode,
		shadeCode:       shadeCode,
		stdWeightFull:   stdFull,
		stdWeightUnfull: stdUnfull,
		notes:           notes,
		createdAt:       time.Now(),
		createdBy:       createdBy,
		source:          SourcePPC,
	}, nil
}

// Reconstruct rebuilds a Master from persistence (no validation).
func Reconstruct(
	lotNo, itemCode, shadeCode string,
	stdFull, stdUnfull float64,
	notes string,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Master {
	return &Master{
		lotNo:           lotNo,
		itemCode:        itemCode,
		shadeCode:       shadeCode,
		stdWeightFull:   stdFull,
		stdWeightUnfull: stdUnfull,
		notes:           notes,
		createdAt:       createdAt,
		createdBy:       createdBy,
		updatedAt:       updatedAt,
		updatedBy:       updatedBy,
	}
}

// WithProvenance attaches sync provenance and the source-owned specification to
// a reconstructed lot.
//
// It is separate from Reconstruct because only persistence sets these, and
// folding four more positional parameters into an already ten-argument
// constructor makes every call site unreadable. Returns the receiver so it can
// be chained onto a Reconstruct call.
func (l *Master) WithProvenance(source, sourceKey string, syncedAt *time.Time, spec Spec) *Master {
	if source == "" {
		source = SourcePPC
	}
	l.source = source
	l.sourceKey = sourceKey
	l.syncedAt = syncedAt
	l.spec = spec
	return l
}

// Source returns the lot's provenance marker (SourcePPC or SourceMMSMERGE).
func (l *Master) Source() string { return l.source }

// SourceKey returns the source system's own key, empty for PPC-minted lots.
func (l *Master) SourceKey() string { return l.sourceKey }

// SyncedAt returns when the sync last touched this lot, nil for PPC-minted lots.
func (l *Master) SyncedAt() *time.Time { return l.syncedAt }

// Spec returns the source-owned yarn and packing specification.
func (l *Master) Spec() Spec { return l.spec }

// IsSourced reports whether the lot came from a sync rather than from PPC.
func (l *Master) IsSourced() bool { return l.source != "" && l.source != SourcePPC }

// LotNo returns the lot number (primary key).
func (l *Master) LotNo() string { return l.lotNo }

// ItemCode returns the item code.
func (l *Master) ItemCode() string { return l.itemCode }

// ShadeCode returns the shade code.
func (l *Master) ShadeCode() string { return l.shadeCode }

// StdWeightFull returns the standard full weight.
func (l *Master) StdWeightFull() float64 { return l.stdWeightFull }

// StdWeightUnfull returns the standard unfull weight.
func (l *Master) StdWeightUnfull() float64 { return l.stdWeightUnfull }

// Notes returns the optional notes.
func (l *Master) Notes() string { return l.notes }

// CreatedAt returns the creation timestamp.
func (l *Master) CreatedAt() time.Time { return l.createdAt }

// CreatedBy returns the creator.
func (l *Master) CreatedBy() string { return l.createdBy }

// UpdatedAt returns the last update timestamp.
func (l *Master) UpdatedAt() *time.Time { return l.updatedAt }

// UpdatedBy returns the last updater.
func (l *Master) UpdatedBy() *string { return l.updatedBy }

// Update applies optional field changes with validation.
func (l *Master) Update(itemCode, shadeCode *string, stdFull, stdUnfull *float64, notes *string, updatedBy string) error {
	if err := l.applyItemCode(itemCode); err != nil {
		return err
	}
	if err := l.applyShadeCode(shadeCode); err != nil {
		return err
	}
	if err := l.applyWeights(stdFull, stdUnfull); err != nil {
		return err
	}
	if notes != nil {
		l.notes = *notes
	}

	now := time.Now()
	l.updatedAt = &now
	l.updatedBy = &updatedBy
	return nil
}

// UpdateSpec replaces the yarn/packing specification wholesale.
//
// A PPC correction to a sync-sourced lot is deliberately allowed: the sync's
// COALESCE merge preserves whatever PPC set, so an operator fixing a wrong
// denier is not undone by the next run. The whole struct is replaced rather
// than field-patched because the spec carries no invariants — a partial patch
// would need thirteen pointers to say nothing extra.
func (l *Master) UpdateSpec(spec Spec, updatedBy string) {
	l.spec = spec
	now := time.Now()
	l.updatedAt = &now
	l.updatedBy = &updatedBy
}

func (l *Master) applyItemCode(itemCode *string) error {
	if itemCode == nil {
		return nil
	}
	if err := validateItemCode(*itemCode); err != nil {
		return err
	}
	l.itemCode = *itemCode
	return nil
}

func (l *Master) applyShadeCode(shadeCode *string) error {
	if shadeCode == nil {
		return nil
	}
	if err := validateShadeCode(*shadeCode); err != nil {
		return err
	}
	l.shadeCode = *shadeCode
	return nil
}

func (l *Master) applyWeights(stdFull, stdUnfull *float64) error {
	if stdFull != nil {
		if *stdFull <= 0 {
			return ErrInvalidWeight
		}
		l.stdWeightFull = *stdFull
	}
	if stdUnfull != nil {
		if *stdUnfull <= 0 {
			return ErrInvalidWeight
		}
		l.stdWeightUnfull = *stdUnfull
	}
	return nil
}

func validateLotNo(lotNo string) error {
	if lotNo == "" {
		return ErrEmptyLotNo
	}
	if len(lotNo) > maxLotNoLen {
		return ErrLotNoTooLong
	}
	return nil
}

func validateItemCode(itemCode string) error {
	if itemCode == "" {
		return ErrEmptyItemCode
	}
	if len(itemCode) > maxItemCodeLen {
		return ErrItemCodeTooLong
	}
	return nil
}

func validateShadeCode(shadeCode string) error {
	if shadeCode == "" {
		return ErrEmptyShadeCode
	}
	if len(shadeCode) > maxShadeCodeLen {
		return ErrShadeCodeTooLong
	}
	return nil
}
