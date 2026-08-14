// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import "strings"

// Maximum lengths for the constrained MB Head recipe fields (spec section 2.1).
const (
	maxMBCostingLen    = 100
	maxMgtNameLen      = 100
	maxDevCodeLen      = 50
	maxVsNumberLen     = 50
	maxNoOfProcessLen  = 10
	maxShadeCodeLen    = 20
	maxShadeNameLen    = 100
	maxCrossSectionLen = 20
	maxFinalProductLen = 200
)

// LDR percentage bounds (spec section 2.2).
const (
	minLdrPercent = 0.0
	maxLdrPercent = 100.0
)

// newBoundedString trims s and enforces a non-empty value no longer than maxLen.
// It is the shared primitive behind every string value object below and behind the
// update path, so a field's bounds are declared exactly once.
func newBoundedString(s string, maxLen int, errEmpty, errTooLong error) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errEmpty
	}
	if len(s) > maxLen {
		return "", errTooLong
	}
	return s, nil
}

// MgtName is the validated MB display name ("MB Name" on the form), 1-100 characters.
type MgtName struct{ value string }

// NewMgtName creates a validated MgtName.
func NewMgtName(s string) (MgtName, error) {
	v, err := newBoundedString(s, maxMgtNameLen, ErrEmptyMgtName, ErrMgtNameTooLong)
	if err != nil {
		return MgtName{}, err
	}
	return MgtName{value: v}, nil
}

// String returns the management name.
func (v MgtName) String() string { return v.value }

// DevCode is the validated development number, 1-50 characters, unique among live records.
type DevCode struct{ value string }

// NewDevCode creates a validated DevCode.
func NewDevCode(s string) (DevCode, error) {
	v, err := newBoundedString(s, maxDevCodeLen, ErrEmptyDevCode, ErrDevCodeTooLong)
	if err != nil {
		return DevCode{}, err
	}
	return DevCode{value: v}, nil
}

// String returns the development code.
func (v DevCode) String() string { return v.value }

// VsNumber is the validated VS number, 1-50 characters, unique among live records.
type VsNumber struct{ value string }

// NewVsNumber creates a validated VsNumber.
func NewVsNumber(s string) (VsNumber, error) {
	v, err := newBoundedString(s, maxVsNumberLen, ErrEmptyVsNumber, ErrVsNumberTooLong)
	if err != nil {
		return VsNumber{}, err
	}
	return VsNumber{value: v}, nil
}

// String returns the VS number.
func (v VsNumber) String() string { return v.value }

// NoOfProcess is the validated number-of-process option code (S, D, T today).
//
// The permitted set is deliberately NOT hardcoded here: per spec section 2.3 the options are
// read live from mst_mb_param_option, so adding a fourth option must not require a domain
// change. The domain enforces only presence and length; membership is enforced by the
// application layer against the live option list.
//
// This is the user-selected header value and is distinct from the frozen
// Entity.ParamNoOfProcess snapshot written by FreezeParams (spec section 2.4).
type NoOfProcess struct{ value string }

// NewNoOfProcess creates a validated NoOfProcess.
func NewNoOfProcess(s string) (NoOfProcess, error) {
	v, err := newBoundedString(s, maxNoOfProcessLen, ErrEmptyNoOfProcess, ErrNoOfProcessTooLong)
	if err != nil {
		return NoOfProcess{}, err
	}
	return NoOfProcess{value: v}, nil
}

// String returns the number-of-process option code.
func (v NoOfProcess) String() string { return v.value }

// ShadeCode is the validated shade code, 1-20 characters.
type ShadeCode struct{ value string }

// NewShadeCode creates a validated ShadeCode.
func NewShadeCode(s string) (ShadeCode, error) {
	v, err := newBoundedString(s, maxShadeCodeLen, ErrEmptyShadeCode, ErrShadeCodeTooLong)
	if err != nil {
		return ShadeCode{}, err
	}
	return ShadeCode{value: v}, nil
}

// String returns the shade code.
func (v ShadeCode) String() string { return v.value }

// ShadeName is the validated shade name, 1-100 characters.
type ShadeName struct{ value string }

// NewShadeName creates a validated ShadeName.
func NewShadeName(s string) (ShadeName, error) {
	v, err := newBoundedString(s, maxShadeNameLen, ErrEmptyShadeName, ErrShadeNameTooLong)
	if err != nil {
		return ShadeName{}, err
	}
	return ShadeName{value: v}, nil
}

// String returns the shade name.
func (v ShadeName) String() string { return v.value }

// CrossSection is the validated cross-section descriptor, 1-20 characters.
type CrossSection struct{ value string }

// NewCrossSection creates a validated CrossSection.
func NewCrossSection(s string) (CrossSection, error) {
	v, err := newBoundedString(s, maxCrossSectionLen, ErrEmptyCrossSection, ErrCrossSectionTooLong)
	if err != nil {
		return CrossSection{}, err
	}
	return CrossSection{value: v}, nil
}

// String returns the cross-section descriptor.
func (v CrossSection) String() string { return v.value }

// FinalProduct is the validated final product description, 1-200 characters.
type FinalProduct struct{ value string }

// NewFinalProduct creates a validated FinalProduct.
func NewFinalProduct(s string) (FinalProduct, error) {
	v, err := newBoundedString(s, maxFinalProductLen, ErrEmptyFinalProduct, ErrFinalProductTooLong)
	if err != nil {
		return FinalProduct{}, err
	}
	return FinalProduct{value: v}, nil
}

// String returns the final product description.
func (v FinalProduct) String() string { return v.value }

// Denier is the validated POY denier, strictly greater than zero.
type Denier struct{ value float64 }

// NewDenier creates a validated Denier.
func NewDenier(f float64) (Denier, error) {
	if f <= 0 {
		return Denier{}, ErrInvalidDenier
	}
	return Denier{value: f}, nil
}

// Float64 returns the denier value.
func (v Denier) Float64() float64 { return v.value }

// Filament is the validated POY filament count, strictly greater than zero.
type Filament struct{ value int }

// NewFilament creates a validated Filament.
func NewFilament(i int) (Filament, error) {
	if i <= 0 {
		return Filament{}, ErrInvalidFilament
	}
	return Filament{value: i}, nil
}

// Int returns the filament count.
func (v Filament) Int() int { return v.value }

// LdrPercent is the validated LDR percentage, within 0-100 inclusive.
type LdrPercent struct{ value float64 }

// NewLdrPercent creates a validated LdrPercent.
func NewLdrPercent(f float64) (LdrPercent, error) {
	if f < minLdrPercent || f > maxLdrPercent {
		return LdrPercent{}, ErrInvalidLdrPercent
	}
	return LdrPercent{value: f}, nil
}

// Float64 returns the LDR percentage.
func (v LdrPercent) Float64() float64 { return v.value }
