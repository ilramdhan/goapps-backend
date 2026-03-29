// Package cms provides domain logic for CMS content management.
package cms

import (
	"errors"
)

// Domain-specific errors for CMS package.
var (
	// Page errors.
	ErrPageNotFound      = errors.New("CMS page not found")
	ErrSlugAlreadyExists = errors.New("page slug already exists")
	ErrInvalidSlugFormat = errors.New("page slug must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens")

	// Section errors.
	ErrSectionNotFound    = errors.New("CMS section not found")
	ErrSectionKeyExists   = errors.New("section key already exists")
	ErrInvalidSectionKey  = errors.New("section key must start with a lowercase letter and contain only lowercase letters, numbers, and underscores")
	ErrInvalidSectionType = errors.New("invalid section type")

	// Setting errors.
	ErrSettingNotFound    = errors.New("CMS setting not found")
	ErrSettingNotEditable = errors.New("this setting is not editable")
)
