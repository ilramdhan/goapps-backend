// Package superbacostsp provides domain logic for the Superba Cost SP master
// (cost_superba_cost_sp): per-shade MB cost marketing for SUPERBA products.
package superbacostsp

import "errors"

// Domain errors for Superba Cost SP operations.
var (
	// ErrNotFound is returned when a row is not found.
	ErrNotFound = errors.New("superba cost sp not found")

	// ErrDuplicateLegacySysID is returned when the legacy sys id already exists.
	ErrDuplicateLegacySysID = errors.New("superba cost sp with this legacy sys id already exists")

	// ErrInvalidLegacySysID is returned when the legacy sys id is not positive.
	ErrInvalidLegacySysID = errors.New("legacy sys id must be positive")

	// ErrEmptyShadeCode is returned when the shade code is empty.
	ErrEmptyShadeCode = errors.New("shade code cannot be empty")

	// ErrShadeCodeTooLong is returned when the shade code exceeds max length.
	ErrShadeCodeTooLong = errors.New("shade code must be at most 30 characters")

	// ErrColourNameTooLong is returned when the colour name exceeds max length.
	ErrColourNameTooLong = errors.New("colour name must be at most 200 characters")

	// ErrNegativeValue is returned when old/new value is negative.
	ErrNegativeValue = errors.New("value cannot be negative")

	// ErrEmptyCreatedBy is returned when created_by is empty.
	ErrEmptyCreatedBy = errors.New("created_by cannot be empty")

	// ErrSyncNotConfigured is returned when a sync is requested but no Oracle
	// source is wired (host unset, unreachable, or source table not configured).
	ErrSyncNotConfigured = errors.New("superba cost sp sync is not configured")
)
