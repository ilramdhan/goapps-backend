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
)

var typeCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)

// CostProductType is the aggregate root. Fields are private; mutation goes through methods.
type CostProductType struct {
	typeID    int32
	typeCode  string
	typeName  string
	isActive  bool
	createdAt time.Time
	updatedAt time.Time
}

// New constructs a new CostProductType (typeID is assigned by the database).
func New(typeCode, typeName string) (*CostProductType, error) {
	typeCode = strings.TrimSpace(typeCode)
	if len(typeCode) == 0 || len(typeCode) > 5 || !typeCodePattern.MatchString(typeCode) {
		return nil, ErrInvalidTypeCode
	}
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return nil, ErrInvalidTypeName
	}
	now := time.Now().UTC()
	return &CostProductType{
		typeCode:  typeCode,
		typeName:  typeName,
		isActive:  true,
		createdAt: now,
		updatedAt: now,
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

// Update mutates name + active flag. type_code is immutable.
func (c *CostProductType) Update(typeName string, isActive bool) error {
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return ErrInvalidTypeName
	}
	c.typeName = typeName
	c.isActive = isActive
	c.updatedAt = time.Now().UTC()
	return nil
}

// SetID assigns the DB-generated typeID (called by the repo after insert).
func (c *CostProductType) SetID(id int32) { c.typeID = id }

// Read-only accessors.
func (c *CostProductType) TypeID() int32       { return c.typeID }
func (c *CostProductType) TypeCode() string    { return c.typeCode }
func (c *CostProductType) TypeName() string    { return c.typeName }
func (c *CostProductType) IsActive() bool      { return c.isActive }
func (c *CostProductType) CreatedAt() time.Time { return c.createdAt }
func (c *CostProductType) UpdatedAt() time.Time { return c.updatedAt }
