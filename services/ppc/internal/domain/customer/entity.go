// Package customer provides domain logic for the PPC customer master.
package customer

import (
	"strings"
	"time"
)

// Field length limits. They are deliberately wider than the Oracle source columns
// (CUST_CODE 12, CUST_NAME 240, CUST_SHORT_NAME 30) so an Orion widening never
// makes the sync fail closed — the DB columns are TEXT for the same reason.
const (
	maxCodeLen      = 30
	maxNameLen      = 240
	maxShortNameLen = 60
	maxTaxNoLen     = 60
)

// Provenance values recorded on customer_source.
const (
	// SourceOracle marks a row created or maintained by the Oracle ETL sync.
	SourceOracle = "ORACLE"
	// SourceManual marks a hand-created row. The Oracle sync never overwrites it.
	SourceManual = "MANUAL"
)

// Customer is the aggregate root for the PPC customer master. Rows are normally
// sync-sourced from Oracle MGTDAT.OM_CUSTOMER, but a planner may also create one
// by hand for a customer Orion does not carry yet.
type Customer struct {
	id              int64
	code            string
	name            string
	shortName       *string
	taxNo           *string
	parentCode      *string
	isActive        bool
	source          string
	sourceCreatedAt *time.Time
	sourceUpdatedAt *time.Time
	syncedAt        *time.Time
	createdAt       time.Time
	createdBy       string
	updatedAt       *time.Time
	updatedBy       *string
}

// NewParams carries the inputs for creating a customer by hand.
type NewParams struct {
	Code       string
	Name       string
	ShortName  *string
	TaxNo      *string
	ParentCode *string
	CreatedBy  string
}

// New creates a hand-authored customer with validation. The code is normalized to
// upper case so it matches the Oracle-sourced rows it shares a unique index with.
func New(p NewParams) (*Customer, error) {
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
	if err := validateOptional(p.TaxNo, maxTaxNoLen, ErrTaxNoTooLong); err != nil {
		return nil, err
	}
	if err := validateOptional(p.ParentCode, maxCodeLen, ErrParentCodeTooLong); err != nil {
		return nil, err
	}
	createdBy := strings.TrimSpace(p.CreatedBy)
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Customer{
		code:       code,
		name:       name,
		shortName:  trimOptional(p.ShortName),
		taxNo:      trimOptional(p.TaxNo),
		parentCode: trimOptional(p.ParentCode),
		isActive:   true,
		source:     SourceManual,
		createdAt:  time.Now(),
		createdBy:  createdBy,
	}, nil
}

// ReconstructParams carries every persisted field for rebuilding an entity.
type ReconstructParams struct {
	ID              int64
	Code            string
	Name            string
	ShortName       *string
	TaxNo           *string
	ParentCode      *string
	IsActive        bool
	Source          string
	SourceCreatedAt *time.Time
	SourceUpdatedAt *time.Time
	SyncedAt        *time.Time
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       *time.Time
	UpdatedBy       *string
}

// Reconstruct rebuilds a Customer from persistence without validation.
func Reconstruct(p ReconstructParams) *Customer {
	return &Customer{
		id:              p.ID,
		code:            p.Code,
		name:            p.Name,
		shortName:       p.ShortName,
		taxNo:           p.TaxNo,
		parentCode:      p.ParentCode,
		isActive:        p.IsActive,
		source:          p.Source,
		sourceCreatedAt: p.SourceCreatedAt,
		sourceUpdatedAt: p.SourceUpdatedAt,
		syncedAt:        p.SyncedAt,
		createdAt:       p.CreatedAt,
		createdBy:       p.CreatedBy,
		updatedAt:       p.UpdatedAt,
		updatedBy:       p.UpdatedBy,
	}
}

// ID returns the customer identifier.
func (c *Customer) ID() int64 { return c.id }

// SetID assigns the generated id (used by the repository after insert).
func (c *Customer) SetID(id int64) { c.id = id }

// Code returns the customer code (the Oracle natural key).
func (c *Customer) Code() string { return c.code }

// Name returns the customer name.
func (c *Customer) Name() string { return c.name }

// ShortName returns the optional short name.
func (c *Customer) ShortName() *string { return c.shortName }

// TaxNo returns the optional tax registration number.
func (c *Customer) TaxNo() *string { return c.taxNo }

// ParentCode returns the optional parent (group head) customer code.
func (c *Customer) ParentCode() *string { return c.parentCode }

// IsActive returns whether the customer is active (not frozen in Orion).
func (c *Customer) IsActive() bool { return c.isActive }

// Source returns the provenance: ORACLE or MANUAL.
func (c *Customer) Source() string { return c.source }

// SourceCreatedAt returns the Oracle-side creation timestamp, if known.
func (c *Customer) SourceCreatedAt() *time.Time { return c.sourceCreatedAt }

// SourceUpdatedAt returns the Oracle-side update timestamp, if known.
func (c *Customer) SourceUpdatedAt() *time.Time { return c.sourceUpdatedAt }

// SyncedAt returns the last successful sync timestamp.
func (c *Customer) SyncedAt() *time.Time { return c.syncedAt }

// CreatedAt returns the creation timestamp.
func (c *Customer) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Customer) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Customer) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Customer) UpdatedBy() *string { return c.updatedBy }

// UpdateParams carries the optional field changes for an edit. The code is not
// mutable: it is the key the Oracle sync upserts on, so renaming it would orphan
// the row from its source.
type UpdateParams struct {
	Name       *string
	ShortName  *string
	TaxNo      *string
	ParentCode *string
	IsActive   *bool
	UpdatedBy  string
}

// Update applies optional field changes with validation.
func (c *Customer) Update(p UpdateParams) error {
	// Parent code has no apply helper, so validate it up front rather than after the
	// other fields have already been mutated.
	if err := validateOptional(p.ParentCode, maxCodeLen, ErrParentCodeTooLong); err != nil {
		return err
	}
	if err := c.applyName(p.Name); err != nil {
		return err
	}
	if err := c.applyShortName(p.ShortName); err != nil {
		return err
	}
	if err := c.applyTaxNo(p.TaxNo); err != nil {
		return err
	}
	if p.ParentCode != nil {
		c.parentCode = trimOptional(p.ParentCode)
	}
	if p.IsActive != nil {
		c.isActive = *p.IsActive
	}

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &p.UpdatedBy
	return nil
}

func (c *Customer) applyName(name *string) error {
	if name == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*name)
	if err := validateName(trimmed); err != nil {
		return err
	}
	c.name = trimmed
	return nil
}

func (c *Customer) applyShortName(shortName *string) error {
	if shortName == nil {
		return nil
	}
	if err := validateOptional(shortName, maxShortNameLen, ErrShortNameTooLong); err != nil {
		return err
	}
	c.shortName = trimOptional(shortName)
	return nil
}

func (c *Customer) applyTaxNo(taxNo *string) error {
	if taxNo == nil {
		return nil
	}
	if err := validateOptional(taxNo, maxTaxNoLen, ErrTaxNoTooLong); err != nil {
		return err
	}
	c.taxNo = trimOptional(taxNo)
	return nil
}

// NormalizeCode trims and upper-cases a customer code so lookups from Oracle, from
// SO staging and from a planner's typing all resolve to the same key.
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
