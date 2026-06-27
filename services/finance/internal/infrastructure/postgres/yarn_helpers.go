package postgres

import (
	"database/sql"
	"time"
)

// nullableFloat64Ptr converts sql.NullFloat64 to *float64.
func nullableFloat64Ptr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// nullableIntPtr converts sql.NullInt64 to *int.
func nullableIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// nullableTimePtr converts sql.NullTime to *time.Time.
func nullableTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	v := n.Time
	return &v
}

// nullableStringPtr converts sql.NullString to *string.
func nullableStringPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// nullableInt32Ptr converts sql.NullInt32 to *int32.
func nullableInt32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}
