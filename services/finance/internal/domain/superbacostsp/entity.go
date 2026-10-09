package superbacostsp

import (
	"strings"
	"time"
)

const (
	maxShadeCodeLen  = 30
	maxColourNameLen = 200
)

// Provenance values recorded on the source column.
const (
	// SourceManual marks a hand-created row.
	SourceManual = "MANUAL"
	// SourceOracle marks a row created or overwritten by the Oracle sync.
	SourceOracle = "ORACLE"
	// SourceSeed marks a row loaded by the CSV seed migration.
	SourceSeed = "SEED"
)

// Entry is the aggregate root: one Superba Cost SP master row.
type Entry struct {
	id          string
	legacySysID int64
	shadeCode   string
	colourName  *string
	oldValue    float64
	newValue    *float64
	source      string
	isActive    bool
	effective   bool
	createdAt   time.Time
	createdBy   string
	updatedAt   *time.Time
	updatedBy   *string
}

// NewParams carries the inputs for creating a row by hand.
type NewParams struct {
	LegacySysID int64
	ShadeCode   string
	ColourName  *string
	OldValue    float64
	NewValue    *float64
	IsActive    bool
	CreatedBy   string
}

// New creates a hand-authored (MANUAL) row with validation.
func New(p NewParams) (*Entry, error) {
	if p.LegacySysID <= 0 {
		return nil, ErrInvalidLegacySysID
	}
	shade := NormalizeShade(p.ShadeCode)
	if err := validateShade(shade); err != nil {
		return nil, err
	}
	colour, err := normalizeColour(p.ColourName)
	if err != nil {
		return nil, err
	}
	if err := validateValues(p.OldValue, p.NewValue); err != nil {
		return nil, err
	}
	createdBy := strings.TrimSpace(p.CreatedBy)
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}
	return &Entry{
		legacySysID: p.LegacySysID,
		shadeCode:   shade,
		colourName:  colour,
		oldValue:    p.OldValue,
		newValue:    p.NewValue,
		source:      SourceManual,
		isActive:    p.IsActive,
		createdAt:   time.Now(),
		createdBy:   createdBy,
	}, nil
}

// ReconstructParams carries every persisted field for rebuilding an entity.
type ReconstructParams struct {
	ID          string
	LegacySysID int64
	ShadeCode   string
	ColourName  *string
	OldValue    float64
	NewValue    *float64
	Source      string
	IsActive    bool
	// Effective is derived: true when this row is the one shade resolution picks.
	Effective bool
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt *time.Time
	UpdatedBy *string
}

// Reconstruct rebuilds an Entry from persistence without validation.
func Reconstruct(p ReconstructParams) *Entry {
	return &Entry{
		id: p.ID, legacySysID: p.LegacySysID, shadeCode: p.ShadeCode,
		colourName: p.ColourName, oldValue: p.OldValue, newValue: p.NewValue,
		source: p.Source, isActive: p.IsActive, effective: p.Effective,
		createdAt: p.CreatedAt, createdBy: p.CreatedBy,
		updatedAt: p.UpdatedAt, updatedBy: p.UpdatedBy,
	}
}

// ID returns the row identifier (UUID).
func (e *Entry) ID() string { return e.id }

// SetID assigns the generated id (used by the repository after insert).
func (e *Entry) SetID(id string) { e.id = id }

// LegacySysID returns the legacy Sys Id (upsert key).
func (e *Entry) LegacySysID() int64 { return e.legacySysID }

// ShadeCode returns the (trimmed, upper-cased) shade code.
func (e *Entry) ShadeCode() string { return e.shadeCode }

// ColourName returns the optional colour name.
func (e *Entry) ColourName() *string { return e.colourName }

// OldValue returns the value used for MB cost marketing.
func (e *Entry) OldValue() float64 { return e.oldValue }

// NewValue returns the informational new value.
func (e *Entry) NewValue() *float64 { return e.newValue }

// Source returns the provenance.
func (e *Entry) Source() string { return e.source }

// IsActive returns whether the row participates in shade resolution.
func (e *Entry) IsActive() bool { return e.isActive }

// Effective reports whether this row is the one resolution picks for its shade.
func (e *Entry) Effective() bool { return e.effective }

// CreatedAt returns the creation timestamp.
func (e *Entry) CreatedAt() time.Time { return e.createdAt }

// CreatedBy returns the creator.
func (e *Entry) CreatedBy() string { return e.createdBy }

// UpdatedAt returns the last update timestamp.
func (e *Entry) UpdatedAt() *time.Time { return e.updatedAt }

// UpdatedBy returns the last updater.
func (e *Entry) UpdatedBy() *string { return e.updatedBy }

// UpdateParams carries optional field changes. legacy_sys_id is immutable (sync key).
type UpdateParams struct {
	ShadeCode  *string
	ColourName *string
	OldValue   *float64
	NewValue   *float64
	IsActive   *bool
	UpdatedBy  string
}

// Update applies optional changes with validation. Any user edit marks the row
// MANUAL; a later Oracle sync overwrites it and sets source back to ORACLE.
func (e *Entry) Update(p UpdateParams) error {
	if p.ShadeCode != nil {
		shade := NormalizeShade(*p.ShadeCode)
		if err := validateShade(shade); err != nil {
			return err
		}
		e.shadeCode = shade
	}
	if p.ColourName != nil {
		colour, err := normalizeColour(p.ColourName)
		if err != nil {
			return err
		}
		e.colourName = colour
	}
	oldV := e.oldValue
	if p.OldValue != nil {
		oldV = *p.OldValue
	}
	newV := e.newValue
	if p.NewValue != nil {
		newV = p.NewValue
	}
	if err := validateValues(oldV, newV); err != nil {
		return err
	}
	e.oldValue, e.newValue = oldV, newV
	if p.IsActive != nil {
		e.isActive = *p.IsActive
	}
	e.source = SourceManual
	now := time.Now()
	e.updatedAt = &now
	e.updatedBy = &p.UpdatedBy
	return nil
}

// NormalizeShade trims and upper-cases a shade code (matches the SQL UPPER(TRIM())).
func NormalizeShade(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func validateShade(shade string) error {
	if shade == "" {
		return ErrEmptyShadeCode
	}
	if len(shade) > maxShadeCodeLen {
		return ErrShadeCodeTooLong
	}
	return nil
}

func normalizeColour(v *string) (*string, error) {
	t := trimOptional(v)
	if t != nil && len(*t) > maxColourNameLen {
		return nil, ErrColourNameTooLong
	}
	return t, nil
}

// trimOptional trims an optional string, mapping empty to nil (NULL column).
func trimOptional(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

func validateValues(oldV float64, newV *float64) error {
	if oldV < 0 || (newV != nil && *newV < 0) {
		return ErrNegativeValue
	}
	return nil
}
