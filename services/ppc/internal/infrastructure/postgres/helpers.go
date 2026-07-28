// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Sort direction and common column constants.
const (
	sortASC  = "ASC"
	sortDESC = "DESC"
)

// isUniqueViolation checks if the error is a PostgreSQL unique violation (23505).
// Handles both lib/pq and pgx/v5/stdlib error types.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	type pgError interface {
		SQLState() string
	}
	var pgErr pgError
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

// isForeignKeyViolation checks if the error is a PostgreSQL FK violation (23503).
func isForeignKeyViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23503"
	}
	type pgError interface {
		SQLState() string
	}
	var pgErr pgError
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23503"
	}
	return false
}

// sortDirection normalizes a caller-supplied sort order to ASC or DESC.
func sortDirection(order string) string {
	if strings.EqualFold(order, sortDESC) {
		return sortDESC
	}
	return sortASC
}

// nullTimePtr converts a sql.NullTime to a *time.Time.
func nullTimePtr(v sql.NullTime) *time.Time {
	if v.Valid {
		return &v.Time
	}
	return nil
}

// nullStringPtr converts a sql.NullString to a *string.
func nullStringPtr(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

// nullFloatPtr converts a sql.NullFloat64 to a *float64.
func nullFloatPtr(v sql.NullFloat64) *float64 {
	if v.Valid {
		return &v.Float64
	}
	return nil
}

// nullInt64Ptr converts a sql.NullInt64 to a *int64.
func nullInt64Ptr(v sql.NullInt64) *int64 {
	if v.Valid {
		return &v.Int64
	}
	return nil
}

// nullInt32Ptr converts a sql.NullInt32 to a *int32.
func nullInt32Ptr(v sql.NullInt32) *int32 {
	if v.Valid {
		return &v.Int32
	}
	return nil
}

// nullBoolPtr converts a sql.NullBool to a *bool.
func nullBoolPtr(v sql.NullBool) *bool {
	if v.Valid {
		return &v.Bool
	}
	return nil
}

// stringPtrToNull converts an optional string to a sql.NullString.
func stringPtrToNull(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}

// boolPtrToNull converts an optional bool to a sql.NullBool.
func boolPtrToNull(v *bool) sql.NullBool {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

// nullString returns the string value of a sql.NullString, or "" when null.
func nullString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// nullableText maps a string to a driver argument, writing an empty string as
// SQL NULL so optional text columns stay NULL rather than storing "".
func nullableText(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// checkAffected returns notFound when an UPDATE/DELETE touched no rows.
func checkAffected(res sql.Result, notFound error) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return notFound
	}
	return nil
}

// int64PtrArg converts an optional int64 to a driver argument. A nil pointer
// yields a typed NULL; pair with an explicit ::BIGINT cast in the SQL when the
// placeholder is reused to avoid SQLSTATE 42P08 (type inference failure).
func int64PtrArg(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// floatPtrArg converts an optional float64 to a driver argument (NULL when nil).
func floatPtrArg(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// timePtrArg converts an optional time.Time to a driver argument (NULL when nil).
func timePtrArg(v *time.Time) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
