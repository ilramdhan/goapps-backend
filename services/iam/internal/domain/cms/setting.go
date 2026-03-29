// Package cms provides domain logic for CMS content management.
package cms

import (
	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// SettingType represents the data type of a CMS setting value.
type SettingType string

// Setting type constants.
const (
	SettingTypeText     SettingType = "TEXT"
	SettingTypeRichText SettingType = "RICH_TEXT"
	SettingTypeImage    SettingType = "IMAGE"
	SettingTypeURL      SettingType = "URL"
	SettingTypeJSON     SettingType = "JSON"
)

// Setting represents a key-value site configuration entity.
type Setting struct {
	id           uuid.UUID
	key          string
	value        string
	settingType  SettingType
	settingGroup string
	description  string
	isEditable   bool
	audit        shared.AuditInfo
}

// ReconstructSetting reconstructs a Setting from persistence.
func ReconstructSetting(
	id uuid.UUID,
	key, value string,
	settingType SettingType,
	settingGroup, description string,
	isEditable bool,
	audit shared.AuditInfo,
) *Setting {
	return &Setting{
		id:           id,
		key:          key,
		value:        value,
		settingType:  settingType,
		settingGroup: settingGroup,
		description:  description,
		isEditable:   isEditable,
		audit:        audit,
	}
}

// ID returns the setting identifier.
func (s *Setting) ID() uuid.UUID { return s.id }

// Key returns the setting key.
func (s *Setting) Key() string { return s.key }

// Value returns the setting value.
func (s *Setting) Value() string { return s.value }

// Type returns the setting type.
func (s *Setting) Type() SettingType { return s.settingType }

// Group returns the setting group.
func (s *Setting) Group() string { return s.settingGroup }

// Description returns the setting description.
func (s *Setting) Description() string { return s.description }

// IsEditable returns whether the setting can be modified.
func (s *Setting) IsEditable() bool { return s.isEditable }

// Audit returns the audit information.
func (s *Setting) Audit() shared.AuditInfo { return s.audit }

// UpdateValue updates the setting value.
func (s *Setting) UpdateValue(value, updatedBy string) error {
	if !s.isEditable {
		return ErrSettingNotEditable
	}
	s.value = value
	s.audit.Update(updatedBy)
	return nil
}
