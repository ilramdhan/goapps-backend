// Package threshold provides domain logic for PPC overrun-threshold-config master data.
package threshold

import "time"

// Allowed threshold levels.
const (
	LevelSystem       = "SYSTEM"
	LevelMachineGroup = "MACHINE_GROUP"
	LevelProductType  = "PRODUCT_TYPE"
	LevelProduct      = "PRODUCT"
	LevelWO           = "WO"
)

// Allowed threshold units.
const (
	UnitPct  = "PCT"
	UnitDoff = "DOFF"
)

// Config is the aggregate root for the overrun-threshold-config master.
type Config struct {
	id           int64
	level        string
	refID        *int64
	unit         string
	warningValue float64
	blockValue   float64
	notes        string
	isActive     bool
	createdAt    time.Time
	createdBy    string
	updatedAt    *time.Time
	updatedBy    *string
}

// NewConfig creates a new Config with validation.
func NewConfig(
	level string,
	refID *int64,
	unit string,
	warningValue, blockValue float64,
	notes, createdBy string,
) (*Config, error) {
	if !IsValidLevel(level) {
		return nil, ErrInvalidLevel
	}
	if !IsValidUnit(unit) {
		return nil, ErrInvalidUnit
	}
	if blockValue < warningValue {
		return nil, ErrInvalidThresholds
	}
	if createdBy == "" {
		return nil, ErrEmptyCreatedBy
	}

	return &Config{
		level:        level,
		refID:        refID,
		unit:         unit,
		warningValue: warningValue,
		blockValue:   blockValue,
		notes:        notes,
		isActive:     true,
		createdAt:    time.Now(),
		createdBy:    createdBy,
	}, nil
}

// Reconstruct rebuilds a Config from persistence (no validation).
func Reconstruct(
	id int64,
	level string,
	refID *int64,
	unit string,
	warningValue, blockValue float64,
	notes string,
	isActive bool,
	createdAt time.Time,
	createdBy string,
	updatedAt *time.Time,
	updatedBy *string,
) *Config {
	return &Config{
		id:           id,
		level:        level,
		refID:        refID,
		unit:         unit,
		warningValue: warningValue,
		blockValue:   blockValue,
		notes:        notes,
		isActive:     isActive,
		createdAt:    createdAt,
		createdBy:    createdBy,
		updatedAt:    updatedAt,
		updatedBy:    updatedBy,
	}
}

// ID returns the config identifier.
func (c *Config) ID() int64 { return c.id }

// Level returns the threshold level.
func (c *Config) Level() string { return c.level }

// RefID returns the scoped reference identifier (nil for SYSTEM).
func (c *Config) RefID() *int64 { return c.refID }

// Unit returns the threshold unit.
func (c *Config) Unit() string { return c.unit }

// WarningValue returns the warning threshold value.
func (c *Config) WarningValue() float64 { return c.warningValue }

// BlockValue returns the block threshold value.
func (c *Config) BlockValue() float64 { return c.blockValue }

// Notes returns the free-text notes.
func (c *Config) Notes() string { return c.notes }

// IsActive returns the active flag.
func (c *Config) IsActive() bool { return c.isActive }

// CreatedAt returns the creation timestamp.
func (c *Config) CreatedAt() time.Time { return c.createdAt }

// CreatedBy returns the creator.
func (c *Config) CreatedBy() string { return c.createdBy }

// UpdatedAt returns the last update timestamp.
func (c *Config) UpdatedAt() *time.Time { return c.updatedAt }

// UpdatedBy returns the last updater.
func (c *Config) UpdatedBy() *string { return c.updatedBy }

// Update applies optional field changes with validation.
func (c *Config) Update(
	unit *string,
	warningValue, blockValue *float64,
	notes *string,
	isActive *bool,
	updatedBy string,
) error {
	if unit != nil {
		if !IsValidUnit(*unit) {
			return ErrInvalidUnit
		}
		c.unit = *unit
	}
	if warningValue != nil {
		c.warningValue = *warningValue
	}
	if blockValue != nil {
		c.blockValue = *blockValue
	}
	if c.blockValue < c.warningValue {
		return ErrInvalidThresholds
	}
	if notes != nil {
		c.notes = *notes
	}
	if isActive != nil {
		c.isActive = *isActive
	}

	now := time.Now()
	c.updatedAt = &now
	c.updatedBy = &updatedBy
	return nil
}

// IsValidLevel reports whether the given level is one of the allowed values.
func IsValidLevel(level string) bool {
	switch level {
	case LevelSystem, LevelMachineGroup, LevelProductType, LevelProduct, LevelWO:
		return true
	default:
		return false
	}
}

// IsValidUnit reports whether the given unit is one of the allowed values.
func IsValidUnit(unit string) bool {
	switch unit {
	case UnitPct, UnitDoff:
		return true
	default:
		return false
	}
}
