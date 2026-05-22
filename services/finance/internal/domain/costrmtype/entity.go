// Package costrmtype contains the CostRmType domain (PRD Phase B §7.2.2, CRMT_).
package costrmtype

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrNotFound is returned when an RM type is not found.
	ErrNotFound = errors.New("cost rm type not found")
	// ErrAlreadyExists is returned when an RM type code already exists.
	ErrAlreadyExists = errors.New("cost rm type code already exists")
	// ErrInvalidTypeCode is returned for invalid type_code.
	ErrInvalidTypeCode = errors.New("invalid type_code")
	// ErrInvalidTypeName is returned for invalid type_name.
	ErrInvalidTypeName = errors.New("invalid type_name")
	// ErrInvalidReferenceTarget is returned for an invalid reference_target value.
	ErrInvalidReferenceTarget = errors.New("invalid reference_target (must be PRODUCT or MASTER)")
)

// ReferenceTarget options.
const (
	ReferenceProduct = "PRODUCT"
	ReferenceMaster  = "MASTER"
)

var typeCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// CostRmType is the RM type aggregate root.
type CostRmType struct {
	typeID           int32
	typeCode         string
	typeName         string
	referenceTarget  string
	allowSubSequence bool
	isActive         bool
	createdAt        time.Time
	updatedAt        time.Time
}

// New constructs a new CostRmType.
func New(typeCode, typeName, referenceTarget string, allowSubSequence bool) (*CostRmType, error) {
	typeCode = strings.TrimSpace(typeCode)
	if len(typeCode) == 0 || len(typeCode) > 20 || !typeCodePattern.MatchString(typeCode) {
		return nil, ErrInvalidTypeCode
	}
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return nil, ErrInvalidTypeName
	}
	referenceTarget = strings.ToUpper(strings.TrimSpace(referenceTarget))
	if referenceTarget != ReferenceProduct && referenceTarget != ReferenceMaster {
		return nil, ErrInvalidReferenceTarget
	}
	now := time.Now().UTC()
	return &CostRmType{
		typeCode:         typeCode,
		typeName:         typeName,
		referenceTarget:  referenceTarget,
		allowSubSequence: allowSubSequence,
		isActive:         true,
		createdAt:        now,
		updatedAt:        now,
	}, nil
}

// Reconstruct rebuilds from persistence.
func Reconstruct(typeID int32, typeCode, typeName, referenceTarget string, allowSubSequence, isActive bool, createdAt, updatedAt time.Time) *CostRmType {
	return &CostRmType{
		typeID:           typeID,
		typeCode:         typeCode,
		typeName:         typeName,
		referenceTarget:  referenceTarget,
		allowSubSequence: allowSubSequence,
		isActive:         isActive,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
	}
}

// Update mutates name + active flag. type_code, reference_target, allow_sub_sequence are immutable.
func (c *CostRmType) Update(typeName string, isActive bool) error {
	typeName = strings.TrimSpace(typeName)
	if len(typeName) == 0 || len(typeName) > 100 {
		return ErrInvalidTypeName
	}
	c.typeName = typeName
	c.isActive = isActive
	c.updatedAt = time.Now().UTC()
	return nil
}

// SetID assigns the DB-generated typeID.
func (c *CostRmType) SetID(id int32) { c.typeID = id }

// Read-only accessors.
func (c *CostRmType) TypeID() int32             { return c.typeID }
func (c *CostRmType) TypeCode() string          { return c.typeCode }
func (c *CostRmType) TypeName() string          { return c.typeName }
func (c *CostRmType) ReferenceTarget() string   { return c.referenceTarget }
func (c *CostRmType) AllowSubSequence() bool    { return c.allowSubSequence }
func (c *CostRmType) IsActive() bool            { return c.isActive }
func (c *CostRmType) CreatedAt() time.Time      { return c.createdAt }
func (c *CostRmType) UpdatedAt() time.Time      { return c.updatedAt }
