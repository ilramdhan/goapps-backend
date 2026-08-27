// Package shade provides domain logic for the shade master (cost_erp_shade),
// R8: a master synced from Oracle MGTDAT.OM_GRADE_CODE_2 with a manual-CRUD
// escape hatch for shades finance needs before Orion carries them.
package shade

import (
	"strings"
	"time"
)

// Field length limits, deliberately wider than the Oracle source columns
// (GRADE_CODE 40, GRADE_NAME 240, GRADE_SHORT_NAME 30) so a future Oracle
// widening never makes the sync fail closed — the DB columns are TEXT for the
// same reason (see migration 000493).
const (
	maxCodeLen      = 60
	maxNameLen      = 300
	maxShortNameLen = 60
)

// Provenance values recorded on ces_shade_source.
const (
	// SourceOracle marks a row created or refreshed by the Oracle ETL sync.
	SourceOracle = "ORACLE"
	// SourceManual marks a hand-created row. The Oracle sync never overwrites it.
	SourceManual = "MANUAL"
)

// Shade is the aggregate root for the finance shade master. Rows are normally
// sync-sourced from Oracle MGTDAT.OM_GRADE_CODE_2, but a finance user may also
// create one by hand for a shade Orion does not carry yet.
type Shade struct {
	id              int64
	code            string
	name            string
	shortName       *string
	isActive        bool
	source          string
	sourceCreatedAt *time.Time
	sourceUpdatedAt *time.Time
	sourceCreatedBy *string
	sourceUpdatedBy *string
	syncedAt        *time.Time
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
}

// NewParams carries the inputs for creating a shade by hand.
type NewParams struct {
	Code      string
	Name      string
	ShortName *string
	CreatedBy string
}

// New creates a hand-authored shade with validation. The code is normalized to
// upper case so it matches the Oracle-sourced rows it shares a unique index with.
func New(p NewParams) (*Shade, error) {
	code := NormalizeCode(p.Code)
	if err := validateCode(code); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(p.Name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateOptional(p.ShortName, maxShortNameLen, ErrShortNameTooLong); err != nil {
		return nil, err
	}
	createdBy := strings.TrimSpace(p.CreatedBy)
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Shade{
		code:      code,
		name:      name,
		shortName: trimOptional(p.ShortName),
		isActive:  true,
		source:    SourceManual,
		createdAt: time.Now(),
		createdBy: createdBy,
	}, nil
}

// ReconstructParams carries every persisted field for rebuilding an entity.
type ReconstructParams struct {
	ID              int64
	Code            string
	Name            string
	ShortName       *string
	IsActive        bool
	Source          string
	SourceCreatedAt *time.Time
	SourceUpdatedAt *time.Time
	SourceCreatedBy *string
	SourceUpdatedBy *string
	SyncedAt        *time.Time
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       *time.Time
	UpdatedBy       *string
}

// Reconstruct rebuilds a Shade from persistence without validation.
func Reconstruct(p ReconstructParams) *Shade {
	return &Shade{
		id:              p.ID,
		code:            p.Code,
		name:            p.Name,
		shortName:       p.ShortName,
		isActive:        p.IsActive,
		source:          p.Source,
		sourceCreatedAt: p.SourceCreatedAt,
		sourceUpdatedAt: p.SourceUpdatedAt,
		sourceCreatedBy: p.SourceCreatedBy,
		sourceUpdatedBy: p.SourceUpdatedBy,
		syncedAt:        p.SyncedAt,
		createdAt:       p.CreatedAt,
		createdBy:       p.CreatedBy,
		updatedAt:       p.UpdatedAt,
		updatedBy:       p.UpdatedBy,
	}
}

// ID returns the shade identifier.
func (s *Shade) ID() int64 { return s.id }

// SetID assigns the generated id (used by the repository after insert).
func (s *Shade) SetID(id int64) { s.id = id }

// Code returns the shade code (the Oracle natural key, GRADE_CODE).
func (s *Shade) Code() string { return s.code }

// Name returns the shade name.
func (s *Shade) Name() string { return s.name }

// ShortName returns the optional short name.
func (s *Shade) ShortName() *string { return s.shortName }

// IsActive returns whether the shade is active (not frozen in Orion).
func (s *Shade) IsActive() bool { return s.isActive }

// Source returns the provenance: ORACLE or MANUAL.
func (s *Shade) Source() string { return s.source }

// SourceCreatedAt returns the Oracle-side creation timestamp, if known.
func (s *Shade) SourceCreatedAt() *time.Time { return s.sourceCreatedAt }

// SourceUpdatedAt returns the Oracle-side update timestamp, if known.
func (s *Shade) SourceUpdatedAt() *time.Time { return s.sourceUpdatedAt }

// SourceCreatedBy returns the Oracle-side creator user id, if known.
func (s *Shade) SourceCreatedBy() *string { return s.sourceCreatedBy }

// SourceUpdatedBy returns the Oracle-side updater user id, if known.
func (s *Shade) SourceUpdatedBy() *string { return s.sourceUpdatedBy }

// SyncedAt returns the last successful sync timestamp.
func (s *Shade) SyncedAt() *time.Time { return s.syncedAt }

// CreatedAt returns the creation timestamp.
func (s *Shade) CreatedAt() time.Time { return s.createdAt }

// CreatedBy returns the creator.
func (s *Shade) CreatedBy() string { return s.createdBy }

// UpdatedAt returns the last update timestamp.
func (s *Shade) UpdatedAt() *time.Time { return s.updatedAt }

// UpdatedBy returns the last updater.
func (s *Shade) UpdatedBy() *string { return s.updatedBy }

// UpdateParams carries the optional field changes for an edit. The code is not
// mutable: it is the key the Oracle sync upserts on, so renaming it would orphan
// the row from its source.
type UpdateParams struct {
	Name      *string
	ShortName *string
	IsActive  *bool
	UpdatedBy string
}

// Update applies optional field changes with validation. It also covers manual
// deactivation: pass IsActive=false to "delete" a shade without losing history.
func (s *Shade) Update(p UpdateParams) error {
	if err := s.applyName(p.Name); err != nil {
		return err
	}
	if err := s.applyShortName(p.ShortName); err != nil {
		return err
	}
	if p.IsActive != nil {
		s.isActive = *p.IsActive
	}

	now := time.Now()
	s.updatedAt = &now
	s.updatedBy = &p.UpdatedBy
	return nil
}

func (s *Shade) applyName(name *string) error {
	if name == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*name)
	if err := validateName(trimmed); err != nil {
		return err
	}
	s.name = trimmed
	return nil
}

func (s *Shade) applyShortName(shortName *string) error {
	if shortName == nil {
		return nil
	}
	if err := validateOptional(shortName, maxShortNameLen, ErrShortNameTooLong); err != nil {
		return err
	}
	s.shortName = trimOptional(shortName)
	return nil
}

// NormalizeCode trims and upper-cases a shade code so lookups from Oracle and
// from a finance user's typing all resolve to the same key.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
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

// validateOptional checks an optional string against a max length. A nil pointer
// means "leave unchanged" and always passes.
func validateOptional(v *string, maxLen int, tooLong error) error {
	if v == nil {
		return nil
	}
	if len(strings.TrimSpace(*v)) > maxLen {
		return tooLong
	}
	return nil
}

// trimOptional trims an optional string, mapping an empty result to nil so the
// column stores NULL rather than an empty string.
func trimOptional(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
