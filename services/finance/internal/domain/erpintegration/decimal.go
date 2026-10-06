// Package erpintegration holds the pure domain model for the GoApps -> ERP
// standard cost integration. Money and rate arithmetic in this package uses
// github.com/shopspring/decimal only; float64/float32 are forbidden by a
// path-scoped forbidigo rule in goapps-backend/.golangci.yml (design §6.1-6.3).
package erpintegration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ScaleR5 is the number of decimal places used by Oracle ROUND(n,5) for the
// standard cost components pushed to ERP.
const ScaleR5 int32 = 5

// ErrInvalidDecimal is returned when a textual decimal cannot be parsed.
var ErrInvalidDecimal = errors.New("erpintegration: invalid decimal value")

// Round5 rounds d to 5 decimal places, half away from zero, matching Oracle
// ROUND(n,5) on NUMBER. shopspring's Decimal.Round is half away from zero;
// banker's rounding (RoundBank) must never be used here.
func Round5(d decimal.Decimal) decimal.Decimal {
	return d.Round(ScaleR5)
}

// ParseDecimal parses a textual decimal (as returned by PostgreSQL NUMERIC or
// Oracle NUMBER text). Surrounding whitespace is ignored. An empty string is
// an error; use ParseNullDecimal for nullable values.
func ParseDecimal(s string) (decimal.Decimal, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return decimal.Zero, fmt.Errorf("%w: empty", ErrInvalidDecimal)
	}
	d, err := decimal.NewFromString(t)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: %q", ErrInvalidDecimal, s)
	}
	return d, nil
}

// ParseNullDecimal parses a nullable textual decimal. A nil pointer or a
// blank string yields an invalid (NULL) NullDecimal without error.
func ParseNullDecimal(s *string) (decimal.NullDecimal, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return decimal.NullDecimal{}, nil
	}
	d, err := ParseDecimal(*s)
	if err != nil {
		return decimal.NullDecimal{}, err
	}
	return decimal.NullDecimal{Decimal: d, Valid: true}, nil
}

// ScanNullDecimal converts a database/sql scan source (string, []byte, int64,
// nil, ...) into a NullDecimal using decimal.NullDecimal.Scan. It exists so
// repositories scan NUMERIC columns straight into decimals, never via float.
func ScanNullDecimal(src any) (decimal.NullDecimal, error) {
	var nd decimal.NullDecimal
	if err := nd.Scan(src); err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("%w: %w", ErrInvalidDecimal, err)
	}
	return nd, nil
}
