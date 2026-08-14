// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import (
	"time"

	"github.com/google/uuid"
)

// Additional-shade sequence bounds. The header carries shade #1; the child table carries
// #2 and #3 only (spec section 4.2).
const (
	// MinShadeSeqNo is the lowest sequence number an additional shade may take.
	MinShadeSeqNo int32 = 2
	// MaxShadeSeqNo is the highest sequence number an additional shade may take.
	MaxShadeSeqNo int32 = 3
	// MaxAdditionalShades is the maximum number of child shade rows per MB head.
	MaxAdditionalShades = 2
)

// Shade is an additional shade attached to an MB head (mst_mb_head_shade), beyond the
// shade carried on the header itself. The type is named Shade rather than MBHeadShade
// because it lives in package mbhead and revive forbids type stuttering.
//
// Additional shades are label-only metadata: they record that one recipe serves several
// shades and have no costing effect (spec section 4.3).
type Shade struct {
	id        uuid.UUID
	mbhID     uuid.UUID
	seqNo     int32
	shadeCode ShadeCode
	shadeName ShadeName
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy *string
	deletedAt *time.Time
	deletedBy *string
}

// NewShade creates a validated additional shade for the given MB head.
func NewShade(mbhID uuid.UUID, seqNo int32, shadeCode, shadeName, createdBy string) (*Shade, error) {
	if seqNo < MinShadeSeqNo || seqNo > MaxShadeSeqNo {
		return nil, ErrInvalidShadeSeqNo
	}
	code, err := NewShadeCode(shadeCode)
	if err != nil {
		return nil, err
	}
	name, err := NewShadeName(shadeName)
	if err != nil {
		return nil, err
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Shade{
		id:        uuid.New(),
		mbhID:     mbhID,
		seqNo:     seqNo,
		shadeCode: code,
		shadeName: name,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// ReconstructShade rebuilds an additional shade from persistence data without re-validating,
// so legacy rows stay readable.
//
//nolint:revive // Many parameters required for persistence reconstitution.
func ReconstructShade(
	id, mbhID uuid.UUID, seqNo int32, shadeCode, shadeName string,
	createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy *string,
	deletedAt *time.Time, deletedBy *string,
) *Shade {
	return &Shade{
		id: id, mbhID: mbhID, seqNo: seqNo,
		shadeCode: ShadeCode{value: shadeCode}, shadeName: ShadeName{value: shadeName},
		createdAt: createdAt, createdBy: createdBy,
		updatedAt: updatedAt, updatedBy: updatedBy,
		deletedAt: deletedAt, deletedBy: deletedBy,
	}
}

// ID returns the shade row's UUID primary key.
func (s *Shade) ID() uuid.UUID { return s.id }

// MBHeadID returns the parent MB head's UUID.
func (s *Shade) MBHeadID() uuid.UUID { return s.mbhID }

// SeqNo returns the display sequence number within the head (2 or 3).
func (s *Shade) SeqNo() int32 { return s.seqNo }

// ShadeCode returns the shade code.
func (s *Shade) ShadeCode() string { return s.shadeCode.String() }

// ShadeName returns the shade name.
func (s *Shade) ShadeName() string { return s.shadeName.String() }

// CreatedAt returns the creation timestamp.
func (s *Shade) CreatedAt() time.Time { return s.createdAt }

// CreatedBy returns the creator.
func (s *Shade) CreatedBy() string { return s.createdBy }

// UpdatedAt returns the last update timestamp, nil if never updated.
func (s *Shade) UpdatedAt() *time.Time { return s.updatedAt }

// UpdatedBy returns the last updater, nil if never updated.
func (s *Shade) UpdatedBy() *string { return s.updatedBy }

// DeletedAt returns the soft-delete timestamp, nil if not deleted.
func (s *Shade) DeletedAt() *time.Time { return s.deletedAt }

// DeletedBy returns who soft-deleted the row, nil if not deleted.
func (s *Shade) DeletedBy() *string { return s.deletedBy }

// IsDeleted returns true if the shade row has been soft-deleted.
func (s *Shade) IsDeleted() bool { return s.deletedAt != nil }

// SetParent assigns the owning MB head, used when shades are built before the head's ID
// is known.
func (s *Shade) SetParent(mbhID uuid.UUID) { s.mbhID = mbhID }

// Update applies a new code and name to an existing shade row.
func (s *Shade) Update(shadeCode, shadeName, updatedBy string) error {
	if s.IsDeleted() {
		return ErrAlreadyDeleted
	}
	code, err := NewShadeCode(shadeCode)
	if err != nil {
		return err
	}
	name, err := NewShadeName(shadeName)
	if err != nil {
		return err
	}
	s.shadeCode = code
	s.shadeName = name
	now := time.Now()
	s.updatedAt = &now
	s.updatedBy = &updatedBy
	return nil
}

// SoftDelete marks the shade row as deleted.
func (s *Shade) SoftDelete(deletedBy string) error {
	if s.IsDeleted() {
		return ErrAlreadyDeleted
	}
	now := time.Now()
	s.deletedAt = &now
	s.deletedBy = &deletedBy
	return nil
}
