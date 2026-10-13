// Package costproducttype contains the CostProductType domain (PRD Phase B §7.2.1, CPT_).
package costproducttype

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrNotFound is returned when a product type is not found.
	ErrNotFound = errors.New("cost product type not found")
	// ErrAlreadyExists is returned when a product type with the same code already exists.
	ErrAlreadyExists = errors.New("cost product type code already exists")
	// ErrInvalidTypeCode is returned for invalid type_code (uppercase alnum, 1-5 chars).
	ErrInvalidTypeCode = errors.New("invalid type_code")
	// ErrInvalidTypeName is returned for invalid type_name (1-100 chars).
	ErrInvalidTypeName = errors.New("invalid type_name")
	// ErrInvalidRMRatioMode is returned for an rm_ratio_mode outside INPUT/DENIER.
	ErrInvalidRMRatioMode = errors.New("rm_ratio_mode must be INPUT or DENIER")
)

// RM ratio modes mirror cost_product_type.cpt_rm_ratio_mode (CHECK constraint).
const (
	// RMRatioModeInput takes the RM ratio from the product-parameter input.
	RMRatioModeInput = "INPUT"
	// RMRatioModeDenier derives the RM ratio from the RM group head denier.
	RMRatioModeDenier = "DENIER"
)

// NormalizeRMRatioMode trims/uppercases v and validates it. Empty yields "" (caller decides default).
func NormalizeRMRatioMode(v string) (string, error) {
	v = strings.ToUpper(strings.TrimSpace(v))
	switch v {
	case "", RMRatioModeInput, RMRatioModeDenier:
		return v, nil
	default:
		return "", ErrInvalidRMRatioMode
	}
}

var typeCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// CostProductType is the aggregate root. Fields are private; mutation goes through methods.
type CostProductType struct {
	typeID    int32
	typeCode  string
	typeName  string
	isActive  bool
	createdAt time.Time
	updatedAt time.Time
	// oilClass is cpt_oil_class: PTY / POY / SUPERBA, "" = no oil.
	oilClass string
	// rmRatioMode is cpt_rm_ratio_mode: INPUT (default) / DENIER.
	rmRatioMode string
}

// New constructs a new CostProductType (typeID is assigned by the database).
// rmRatioMode empty defaults to INPUT.
func New(typeCode, typeName, rmRatioMode string) (*CostProductType, error) {
	typeCode = strings.TrimSpace(typeCode)
	if len(typeCode) == 0 || len(typeCode) > 5 || !typeCodePattern.MatchString(typeCode) {
		return nil, ErrInvalidTypeCode
	}
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return nil, ErrInvalidTypeName
	}
	mode, err := NormalizeRMRatioMode(rmRatioMode)
	if err != nil {
		return nil, err
	}
	if mode == "" {
		mode = RMRatioModeInput
	}
	now := time.Now().UTC()
	return &CostProductType{
		rmRatioMode: mode,
		typeCode:    typeCode,
		typeName:    typeName,
		isActive:    true,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// Reconstruct rebuilds an entity from persistence (no validation).
func Reconstruct(typeID int32, typeCode, typeName string, isActive bool, createdAt, updatedAt time.Time) *CostProductType {
	return &CostProductType{
		typeID:    typeID,
		typeCode:  typeCode,
		typeName:  typeName,
		isActive:  isActive,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
}

// Update mutates name, active flag and rm_ratio_mode (empty = keep existing).
// type_code is immutable.
func (c *CostProductType) Update(typeName string, isActive bool, rmRatioMode string) error {
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return ErrInvalidTypeName
	}
	mode, err := NormalizeRMRatioMode(rmRatioMode)
	if err != nil {
		return err
	}
	if mode != "" {
		c.rmRatioMode = mode
	}
	c.typeName = typeName
	c.isActive = isActive
	c.updatedAt = time.Now().UTC()
	return nil
}

// SetID assigns the DB-generated typeID (called by the repo after insert).
func (c *CostProductType) SetID(id int32) { c.typeID = id }

// TypeID returns the type ID.
func (c *CostProductType) TypeID() int32 { return c.typeID }

// TypeCode returns the type code.
func (c *CostProductType) TypeCode() string { return c.typeCode }

// TypeName returns the type name.
func (c *CostProductType) TypeName() string { return c.typeName }

// IsActive returns the is active.
func (c *CostProductType) IsActive() bool { return c.isActive }

// CreatedAt returns the created at.
func (c *CostProductType) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the updated at.
func (c *CostProductType) UpdatedAt() time.Time { return c.updatedAt }

// OilClass returns the oil class (PTY / POY / SUPERBA), "" when the type has no oil.
func (c *CostProductType) OilClass() string { return c.oilClass }

// SetOilClass sets the oil class on hydration (repositories only). Changes go
// through the oil-config use case, which validates the class and groups together.
func (c *CostProductType) SetOilClass(v string) { c.oilClass = v }

// RMRatioMode returns INPUT or DENIER (INPUT when unset).
func (c *CostProductType) RMRatioMode() string {
	if c.rmRatioMode == "" {
		return RMRatioModeInput
	}
	return c.rmRatioMode
}

// SetRMRatioMode sets the mode on hydration (repositories only).
func (c *CostProductType) SetRMRatioMode(v string) { c.rmRatioMode = v }
